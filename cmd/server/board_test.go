package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"offline-inspection-board/internal/board"
)

func (a api) get(t *testing.T, cookie *http.Cookie, path string) response {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res, err := a.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{status: res.StatusCode, body: body}
}

func (a api) list(t *testing.T, supervisor *http.Cookie, query string) []board.Row {
	t.Helper()
	res := a.get(t, supervisor, "/api/board"+query)
	res.expect(t, http.StatusOK)
	var rows []board.Row
	if err := json.Unmarshal(res.body, &rows); err != nil || rows == nil {
		t.Fatalf("보드 목록 응답 불일치: %s (%v)", res.body, err)
	}
	return rows
}

func pumpRow(rootID string, revision int32, result, memo string, receivedAt time.Time) board.Row {
	return board.Row{
		RootID: rootID, Revision: revision, EquipmentID: "P-12", EquipmentName: "냉각수 펌프",
		Result: result, Memo: memo, AuthorID: "worker-1", AuthorName: "직원 1",
		InspectedAt: 1764130000000, ReceivedAt: receivedAt,
	}
}

func TestBoardOrderAndReplays(t *testing.T) {
	a := startAPI(t)
	worker := a.login(t, "worker-1")
	supervisor := a.login(t, "supervisor")
	priorID, lateID, laterID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	priorBody := strings.Replace(normal(priorID, 0), "1764130000000", "1764130000001", 1)
	prior := a.save(t, worker, priorID, priorBody)
	original := normal(lateID, 0)
	late := a.save(t, worker, lateID, original)
	wantLate := pumpRow(lateID, 0, "normal", "", late.ReceivedAt)
	wantPrior := pumpRow(priorID, 0, "normal", "", prior.ReceivedAt)
	wantPrior.InspectedAt++
	rows := a.list(t, supervisor, "")
	if len(rows) < 2 || rows[0] != wantLate || rows[1] != wantPrior {
		t.Fatalf("뒤늦게 접수된 점검이 목록의 첫 행이어야 한다: %+v", rows)
	}

	correctionID := uuid.NewString()
	correction := payload(priorID, 1, "P-12", "abnormal", "누수")
	corrected := a.save(t, worker, correctionID, correction)
	wantCorrection := pumpRow(priorID, 1, "abnormal", "누수", corrected.ReceivedAt)
	updated := a.list(t, supervisor, "?filter=all")
	if len(updated) != len(rows) || updated[0] != wantCorrection || updated[1] != wantLate {
		t.Fatalf("정정한 점검이 목록의 첫 행으로 이동해야 한다: %+v", updated)
	}

	a.save(t, worker, laterID, normal(laterID, 0))
	before := a.list(t, supervisor, "")
	for _, retry := range []putRequest{{id: correctionID, body: correction}, {id: priorID, body: priorBody}} {
		a.submit(t, worker, retry.id, retry.body).expect(t, http.StatusOK)
		if after := a.list(t, supervisor, ""); !slices.Equal(before, after) {
			t.Fatalf("재전송으로 보드 순서 또는 접수 시각이 바뀌었다: 전 %+v, 후 %+v", before, after)
		}
	}
}

func TestBoardAbnormalFilter(t *testing.T) {
	a := startAPI(t)
	worker := a.login(t, "worker-1")
	supervisor := a.login(t, "supervisor")
	rootID := uuid.NewString()
	first := a.save(t, worker, rootID, payload(rootID, 0, "P-12", "abnormal", "누수"))
	want := pumpRow(rootID, 0, "abnormal", "누수", first.ReceivedAt)
	if rows := a.list(t, supervisor, "?filter=abnormal"); !slices.Contains(rows, want) {
		t.Fatalf("이상 필터에 점검이 없다: %+v", rows)
	}
	second := a.save(t, worker, uuid.NewString(), normal(rootID, 1))
	for _, row := range a.list(t, supervisor, "?filter=abnormal") {
		if row.RootID == rootID {
			t.Fatalf("정상으로 정정한 점검이 이상 필터에 남아 있다: %+v", row)
		}
	}
	want = pumpRow(rootID, 1, "normal", "", second.ReceivedAt)
	if rows := a.list(t, supervisor, "?filter=all"); !slices.Contains(rows, want) {
		t.Fatalf("전체 필터에 정정한 점검이 없다: %+v", rows)
	}
	for _, query := range []string{"?filter=normal", "?filter=", "?filter=all&filter=abnormal"} {
		a.get(t, supervisor, "/api/board"+query).expect(t, http.StatusUnprocessableEntity)
	}
}

func TestBoardDetail(t *testing.T) {
	a := startAPI(t)
	worker := a.login(t, "worker-1")
	supervisor := a.login(t, "supervisor")
	rootID, firstID, latestID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	original := a.save(t, worker, rootID, normal(rootID, 0))
	first := a.save(t, worker, firstID, payload(rootID, 1, "P-12", "abnormal", "누수"))
	latest := a.save(t, worker, latestID, normal(rootID, 2))
	type revision struct {
		ID string `json:"id"`
		board.Row
	}
	wantHistory := []revision{
		{rootID, pumpRow(rootID, 0, "normal", "", original.ReceivedAt)},
		{firstID, pumpRow(rootID, 1, "abnormal", "누수", first.ReceivedAt)},
		{latestID, pumpRow(rootID, 2, "normal", "", latest.ReceivedAt)},
	}
	res := a.get(t, supervisor, "/api/board/"+rootID)
	res.expect(t, http.StatusOK)
	var detail struct {
		Current board.Row  `json:"current"`
		History []revision `json:"history"`
	}
	if err := json.Unmarshal(res.body, &detail); err != nil || detail.Current != wantHistory[2].Row ||
		!slices.Equal(detail.History, wantHistory) {
		t.Fatalf("보드 상세 응답 불일치: %s (%v)", res.body, err)
	}
	for _, id := range []string{uuid.NewString(), "invalid", firstID} {
		a.get(t, supervisor, "/api/board/"+id).expect(t, http.StatusNotFound)
	}
}

func TestBoardAccess(t *testing.T) {
	a := startAPI(t)
	worker := a.login(t, "worker-1")
	for _, path := range []string{"/api/board", "/api/board/" + uuid.NewString(), "/api/board/events"} {
		t.Run(path, func(t *testing.T) {
			a.get(t, worker, path).expect(t, http.StatusForbidden)
			a.get(t, nil, path).expect(t, http.StatusUnauthorized)
		})
	}
}

func (a api) openBoardStream(t *testing.T, supervisor *http.Cookie) *bufio.Reader {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL+"/api/board/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(supervisor)
	res, err := a.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("SSE 연결 응답 불일치: HTTP %d, Content-Type %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	return bufio.NewReader(res.Body)
}

func readSSEFrame(t *testing.T, stream *bufio.Reader) string {
	t.Helper()
	var frame strings.Builder
	for {
		line, err := stream.ReadString('\n')
		if err != nil {
			t.Fatalf("SSE 프레임 읽기 실패: %v", err)
		}
		if line == "\n" {
			return frame.String()
		}
		frame.WriteString(line)
	}
}

func (a api) expectStreamRow(t *testing.T, supervisor *http.Cookie, stream *bufio.Reader, want board.Row) {
	t.Helper()
	frame := readSSEFrame(t, stream)
	for frame == ":\n" {
		frame = readSSEFrame(t, stream)
	}
	data, ok := strings.CutPrefix(frame, "event: row\ndata: ")
	var row board.Row
	if err := json.Unmarshal([]byte(data), &row); !ok || err != nil || row != want {
		t.Fatalf("SSE 알림 %q, 기대 %+v", frame, want)
	}
	if rows := a.list(t, supervisor, ""); len(rows) == 0 || rows[0] != row {
		t.Fatalf("SSE 알림과 보드 첫 행이 다르다: 알림 %+v, 목록 %+v", row, rows)
	}
}
