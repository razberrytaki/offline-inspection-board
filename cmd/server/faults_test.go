package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDroppedInspectionResponse(t *testing.T) {
	t.Setenv("FAULT_INJECTION", "1")
	a := startAPI(t)
	cookie := a.login(t, "worker-1")
	events, unsubscribe := a.hub.Subscribe()
	defer unsubscribe()

	initial := readFaults(t, a)
	setFaults(t, a, `{"dropAfterCommit":1}`, http.StatusNoContent)

	id := uuid.NewString()
	body := normal(id, 0)
	if dropped, err := a.put(t.Context(), cookie, id, body); err == nil || dropped.status != 0 {
		t.Fatalf("응답이 끊기지 않았다: HTTP %d, 오류 %v", dropped.status, err)
	}

	var firstReceivedAt time.Time
	if err := testPool.QueryRow(t.Context(), "SELECT received_at FROM inspections WHERE id = $1", id).Scan(&firstReceivedAt); err != nil {
		t.Fatal(err)
	}
	expectRows(t, id, 1)
	wantEvent := pumpRow(id, 0, "normal", "", firstReceivedAt.UTC())
	expectOnlyEvent(t, events, wantEvent)

	retry := a.submit(t, cookie, id, body)
	retry.expect(t, http.StatusOK)
	if got := retry.receipt(t, id).ReceivedAt; !got.Equal(firstReceivedAt) {
		t.Fatalf("재전송 접수 시각 %v, 기대 %v", got, firstReceivedAt)
	}
	expectRows(t, id, 1)
	select {
	case event := <-events:
		t.Fatalf("재전송에서 추가 접수 알림 발생: %+v", event)
	default:
	}
	state := readFaults(t, a)
	if state.DropAfterCommit != 0 || state.InspectionCount != initial.InspectionCount+1 || len(state.Log) != 2 {
		t.Fatalf("장애 상태: %+v", state)
	}
	for i, want := range []string{`"dropped"`, `200`} {
		if entry := state.Log[i]; entry.ID != id || entry.At.IsZero() || string(entry.Outcome) != want {
			t.Fatalf("요청 로그 %d: %+v", i, entry)
		}
	}

}

func TestFaultRouteDisabled(t *testing.T) {
	t.Setenv("FAULT_INJECTION", "")
	a := startAPI(t)
	setFaults(t, a, `{"dropAfterCommit":1}`, http.StatusNotFound)
	for _, path := range []string{"/api/dev", "/api/dev/", "/api/dev/faults", "/api/dev/unknown"} {
		res, err := a.Client().Get(a.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		response{status: res.StatusCode}.expect(t, http.StatusNotFound)
	}

	id := uuid.NewString()
	a.save(t, a.login(t, "worker-1"), id, normal(id, 0))
}

func TestOfflineCutsAPIAndOpenBoardStream(t *testing.T) {
	t.Setenv("FAULT_INJECTION", "1")
	a := startAPI(t)
	worker := a.login(t, "worker-1")
	supervisor := a.login(t, "supervisor")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL+"/api/board/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(supervisor)
	stream, err := a.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	response{status: stream.StatusCode}.expect(t, http.StatusOK)
	initial := readFaults(t, a)

	setFaults(t, a, `{"dropAfterCommit":1}`, http.StatusNoContent)
	for _, invalid := range []string{`{"offline":true,"other":1}`, `{"offline":null}`, `{"dropAfterCommit":-1}`, `null`} {
		setFaults(t, a, invalid, http.StatusBadRequest)
	}
	if state := readFaults(t, a); state.Offline || state.DropAfterCommit != 1 {
		t.Fatalf("잘못된 요청으로 장애 상태가 변경됐다: %+v", state)
	}
	setFaults(t, a, `{"offline":true}`, http.StatusNoContent)
	if _, err := io.Copy(io.Discard, stream.Body); err == nil || ctx.Err() != nil {
		t.Fatalf("열린 SSE 연결이 끊기지 않았다: %v (context %v)", err, ctx.Err())
	}
	for _, path := range []string{"/api/me", "/api/board/events", "/api/unknown"} {
		if res, err := a.Client().Get(a.URL + path); err == nil {
			res.Body.Close()
			t.Fatalf("%s: 오프라인인데 HTTP %d 응답을 받았다", path, res.StatusCode)
		}
	}
	id := uuid.NewString()
	if res, err := a.put(ctx, worker, id, normal(id, 0)); err == nil {
		t.Fatalf("오프라인인데 점검 PUT에 HTTP %d 응답을 받았다", res.status)
	}
	state := readFaults(t, a)
	if !state.Offline || state.DropAfterCommit != 1 || state.InspectionCount != initial.InspectionCount ||
		len(state.Log) != 1 || state.Log[0].ID != id || string(state.Log[0].Outcome) != `"offline"` {
		t.Fatalf("장애 상태: %+v", state)
	}

	setFaults(t, a, `{"offline":false,"dropAfterCommit":0}`, http.StatusNoContent)
	a.save(t, worker, id, normal(id, 0))
}

func TestFaultLogKeepsLastTwentyAttempts(t *testing.T) {
	t.Setenv("FAULT_INJECTION", "1")
	a := startAPI(t)
	cookie := a.login(t, "worker-1")
	ids := make([]string, 22)
	for i := range ids {
		ids[i] = uuid.NewString()
		a.submit(t, cookie, ids[i], `{}`).expect(t, http.StatusUnprocessableEntity)
	}
	log := readFaults(t, a).Log
	if len(log) != 20 {
		t.Fatalf("로그에 %d개 요청이 남았다", len(log))
	}
	for i, entry := range log {
		if entry.ID != ids[i+2] || string(entry.Outcome) != "422" {
			t.Fatalf("로그 항목 %d: %+v", i, entry)
		}
	}
}

func setFaults(t *testing.T, a api, body string, status int) {
	t.Helper()
	res, err := a.Client().Post(a.URL+"/api/dev/faults", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	response{status: res.StatusCode}.expect(t, status)
}

type faultStateResponse struct {
	DropAfterCommit int
	Offline         bool
	InspectionCount int
	Log             []struct {
		At      time.Time
		ID      string
		Outcome json.RawMessage
	}
}

func readFaults(t *testing.T, a api) faultStateResponse {
	t.Helper()
	res, err := a.Client().Get(a.URL + "/api/dev/faults")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	response{status: res.StatusCode}.expect(t, http.StatusOK)
	var state faultStateResponse
	if err := json.NewDecoder(res.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	return state
}
