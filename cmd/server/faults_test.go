package main

import (
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

	res, err := a.Client().Post(a.URL+"/api/dev/faults", "application/json", strings.NewReader(`{"dropAfterCommit":1}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	response{status: res.StatusCode}.expect(t, http.StatusNoContent)

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
}

func TestFaultRouteDisabled(t *testing.T) {
	t.Setenv("FAULT_INJECTION", "")
	a := startAPI(t)
	res, err := a.Client().Post(a.URL+"/api/dev/faults", "application/json", strings.NewReader(`{"dropAfterCommit":1}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	response{status: res.StatusCode}.expect(t, http.StatusNotFound)

	id := uuid.NewString()
	a.save(t, a.login(t, "worker-1"), id, normal(id, 0))
}
