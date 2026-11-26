package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"offline-inspection-board/internal/board"
	"offline-inspection-board/internal/db"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) (code int) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:dev@127.0.0.1:5433/oib?sslmode=disable"
	}
	target, err := url.Parse(databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "테스트 DB 주소 확인 실패:", err)
		return 1
	}
	if target.Scheme != "postgres" && target.Scheme != "postgresql" {
		fmt.Fprintln(os.Stderr, "TEST_DATABASE_URL에는 PostgreSQL URL을 설정하세요")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "테스트 DB 서버에 연결 실패:", err)
		return 1
	}
	defer admin.Close(context.Background())

	name := "oib_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		fmt.Fprintln(os.Stderr, "테스트 DB 생성 실패:", err)
		return 1
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+identifier); err != nil {
			fmt.Fprintln(os.Stderr, "테스트 DB 삭제 실패:", err)
			code = 1
		}
	}()

	target.Path = "/" + name
	query := target.Query()
	query.Del("database")
	query.Del("dbname")
	query.Set("pool_max_conns", "8")
	target.RawQuery = query.Encode()
	testPool, err = db.Open(ctx, target.String())
	if err != nil {
		fmt.Fprintln(os.Stderr, "테스트 DB 초기화 실패:", err)
		return 1
	}
	defer testPool.Close()
	return m.Run()
}

type api struct {
	*httptest.Server
	hub *board.Hub
}

func startAPI(t *testing.T) api {
	t.Helper()
	hub := &board.Hub{}
	server := httptest.NewServer(routes(testPool, hub, 25*time.Millisecond))
	t.Cleanup(server.Close)
	return api{Server: server, hub: hub}
}

type response struct {
	status int
	body   []byte
}

func (r response) expect(t *testing.T, status int) {
	t.Helper()
	if r.status != status {
		t.Fatalf("HTTP %d, 기대 %d, 본문 %s", r.status, status, r.body)
	}
}

type receipt struct {
	ID         string    `json:"id"`
	ReceivedAt time.Time `json:"receivedAt"`
}

func (r response) receipt(t *testing.T, id string) receipt {
	t.Helper()
	var value receipt
	if err := json.Unmarshal(r.body, &value); err != nil {
		t.Fatalf("접수 응답 해석 실패: %v, 본문 %s", err, r.body)
	}
	if value.ID != id || value.ReceivedAt.IsZero() {
		t.Fatalf("접수 응답 불일치: %s", r.body)
	}
	return value
}

func (r response) conflict(t *testing.T, reason string) json.RawMessage {
	t.Helper()
	r.expect(t, http.StatusConflict)
	var value struct {
		Reason  string          `json:"reason"`
		Current json.RawMessage `json:"current"`
	}
	if err := json.Unmarshal(r.body, &value); err != nil || value.Reason != reason {
		t.Fatalf("충돌 사유 %q 기대, 본문 %s", reason, r.body)
	}
	if reason != "content_mismatch" && value.Current != nil {
		t.Fatalf("이 충돌에는 저장된 기록을 반환하지 않는다: %s", r.body)
	}
	return value.Current
}

func (a api) login(t *testing.T, userID string) *http.Cookie {
	t.Helper()
	body := fmt.Sprintf(`{"userId":%q,"password":%q}`, userID, "demo-"+userID)
	res, err := a.Client().Post(a.URL+"/session", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	cookies := res.Cookies()
	if res.StatusCode != http.StatusNoContent || len(cookies) != 1 {
		t.Fatalf("로그인 HTTP %d, 쿠키 %d개", res.StatusCode, len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "session" || cookie.Value == "" || !cookie.HttpOnly || time.Until(cookie.Expires) < 12*time.Hour {
		t.Fatal("세션 쿠키 이름, 값, HttpOnly 또는 만료 시간 불일치")
	}
	return cookie
}

func (a api) put(ctx context.Context, cookie *http.Cookie, id, body string) (response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, a.URL+"/inspections/"+id, strings.NewReader(body))
	if err != nil {
		return response{}, err
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := a.Client().Do(req)
	if err != nil {
		return response{}, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	return response{status: res.StatusCode, body: data}, err
}

func (a api) submit(t *testing.T, cookie *http.Cookie, id, body string) response {
	t.Helper()
	res, err := a.put(t.Context(), cookie, id, body)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (a api) save(t *testing.T, cookie *http.Cookie, id, body string) receipt {
	t.Helper()
	res := a.submit(t, cookie, id, body)
	res.expect(t, http.StatusCreated)
	return res.receipt(t, id)
}

func payload(rootID string, revision int, equipment, result, memo string) string {
	return fmt.Sprintf(`{"rootId":%q,"revision":%d,"equipmentId":%q,"result":%q,"memo":%q,"inspectedAt":1764130000000}`,
		rootID, revision, equipment, result, memo)
}

func normal(rootID string, revision int) string {
	return payload(rootID, revision, "P-12", "normal", "")
}

func expectRows(t *testing.T, rootID string, want int) {
	t.Helper()
	var count int
	err := testPool.QueryRow(t.Context(), "SELECT count(*) FROM inspections WHERE root_id = $1", rootID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("저장된 기록 %d건, 기대 %d건", count, want)
	}
}

func expectOnlyEvent(t *testing.T, events <-chan board.Row, want board.Row) {
	t.Helper()
	select {
	case event := <-events:
		if event != want {
			t.Fatalf("접수 알림 %+v, 기대 %+v", event, want)
		}
	default:
		t.Fatal("접수 알림이 없다")
	}
	select {
	case event := <-events:
		t.Fatalf("추가 접수 알림: %+v", event)
	default:
	}
}

func TestFirstSave(t *testing.T) {
	a := startAPI(t)
	cookie := a.login(t, "worker-1")
	for _, equipment := range []string{"P-12", "P-03"} {
		t.Run(equipment, func(t *testing.T) {
			id := uuid.NewString()
			a.save(t, cookie, id, payload(id, 0, equipment, "abnormal", "가동 중 누수 발견"))
			expectRows(t, id, 1)
		})
	}
}

func TestReplay(t *testing.T) {
	a := startAPI(t)
	cookie := a.login(t, "worker-1")
	id := uuid.NewString()
	request := normal(id, 0)
	first := a.save(t, cookie, id, request)
	retry := a.submit(t, cookie, id, request)
	retry.expect(t, http.StatusOK)
	if got := retry.receipt(t, id); got != first {
		t.Fatalf("최초 접수와 재전송 응답 불일치: 최초 %+v, 재전송 %+v", first, got)
	}
	expectRows(t, id, 1)
}

func TestReplayAfterLostResponse(t *testing.T) {
	hub := &board.Hub{}
	handler := routes(testPool, hub, 15*time.Second)
	firstResponse := make(chan response, 1)
	var disconnected atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !disconnected.CompareAndSwap(false, true) {
			handler.ServeHTTP(w, r)
			return
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, r)
		firstResponse <- response{status: recorder.Code, body: recorder.Body.Bytes()}

		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	}))
	t.Cleanup(server.Close)
	a := api{Server: server, hub: hub}
	cookie := a.login(t, "worker-1")
	events, unsubscribe := hub.Subscribe()
	t.Cleanup(unsubscribe)

	id := uuid.NewString()
	body := normal(id, 0)
	if _, err := a.put(t.Context(), cookie, id, body); err == nil {
		t.Fatal("연결을 닫은 뒤에도 첫 응답을 받았다")
	}
	var accepted receipt
	select {
	case first := <-firstResponse:
		first.expect(t, http.StatusCreated)
		accepted = first.receipt(t, id)
		retry := a.submit(t, cookie, id, body)
		retry.expect(t, http.StatusOK)
		if !bytes.Equal(first.body, retry.body) {
			t.Fatalf("응답 유실 전후의 접수 응답 불일치: %s, %s", first.body, retry.body)
		}
	default:
		t.Fatal("연결을 닫기 전에 서버가 요청을 저장하지 않았다")
	}
	expectRows(t, id, 1)
	expectOnlyEvent(t, events, pumpRow(id, 0, "normal", "", accepted.ReceivedAt))
}

func TestContentMismatch(t *testing.T) {
	a := startAPI(t)
	author := a.login(t, "worker-1")
	other := a.login(t, "worker-2")
	id := uuid.NewString()
	request := payload(id, 0, "P-12", "abnormal", " 누수 ")
	first := a.save(t, author, id, request)
	for _, tc := range []struct {
		name   string
		cookie *http.Cookie
		body   string
	}{
		{"DifferentMemo", author, payload(id, 0, "P-12", "abnormal", "진동")},
		{"DifferentResult", author, payload(id, 0, "P-12", "normal", "누수")},
		{"DifferentTime", author, strings.Replace(request, "1764130000000", "1764130000001", 1)},
		{"DifferentEquipment", author, payload(id, 0, "V-04", "abnormal", "누수")},
		{"UnknownEquipment", author, payload(id, 0, "missing", "abnormal", "누수")},
		{"DifferentAuthor", other, request},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := a.submit(t, tc.cookie, id, tc.body)
			current := res.conflict(t, "content_mismatch")
			if tc.cookie == other {
				if current != nil {
					t.Fatalf("다른 사용자에게 저장된 기록을 노출했다: %s", current)
				}
				return
			}
			var saved struct {
				receipt
				RootID      string `json:"rootId"`
				Revision    int32  `json:"revision"`
				EquipmentID string `json:"equipmentId"`
				Result      string `json:"result"`
				Memo        string `json:"memo"`
				InspectedAt int64  `json:"inspectedAt"`
				AuthorID    string `json:"authorId"`
			}
			saved.Revision = -1
			if err := json.Unmarshal(current, &saved); err != nil {
				t.Fatal(err)
			}
			if saved.receipt != first || saved.RootID != id || saved.Revision != 0 ||
				saved.EquipmentID != "P-12" || saved.Result != "abnormal" || saved.Memo != "누수" ||
				saved.InspectedAt != 1764130000000 || saved.AuthorID != "worker-1" {
				t.Fatalf("저장된 원본 응답 불일치: %s", current)
			}
		})
	}
	expectRows(t, id, 1)
}

func TestRevisionTaken(t *testing.T) {
	a := startAPI(t)
	firstDevice := a.login(t, "worker-1")
	secondDevice := a.login(t, "worker-1")
	rootID := uuid.NewString()
	a.save(t, firstDevice, rootID, normal(rootID, 0))
	a.save(t, firstDevice, uuid.NewString(), normal(rootID, 1))
	a.submit(t, secondDevice, uuid.NewString(), normal(rootID, 1)).conflict(t, "revision_taken")
	expectRows(t, rootID, 2)
}

func TestMissingPrevious(t *testing.T) {
	a := startAPI(t)
	cookie := a.login(t, "worker-1")
	t.Run("MissingRoot", func(t *testing.T) {
		a.submit(t, cookie, uuid.NewString(), normal(uuid.NewString(), 1)).conflict(t, "missing_previous")
	})
	t.Run("RevisionGap", func(t *testing.T) {
		rootID := uuid.NewString()
		a.save(t, cookie, rootID, normal(rootID, 0))
		a.submit(t, cookie, uuid.NewString(), normal(rootID, 2)).conflict(t, "missing_previous")
		expectRows(t, rootID, 1)
	})
}

func TestUnauthorized(t *testing.T) {
	a := startAPI(t)
	id := uuid.NewString()
	request := normal(id, 0)
	t.Run("NoSession", func(t *testing.T) {
		a.submit(t, nil, id, request).expect(t, http.StatusUnauthorized)
	})
	t.Run("ExpiredSession", func(t *testing.T) {
		cookie := a.login(t, "worker-1")
		_, err := testPool.Exec(t.Context(), "UPDATE sessions SET expires_at = now() - interval '1 second' WHERE token = $1", cookie.Value)
		if err != nil {
			t.Fatal(err)
		}
		a.submit(t, cookie, id, request).expect(t, http.StatusUnauthorized)
	})
	expectRows(t, id, 0)
}

func TestForbiddenCorrection(t *testing.T) {
	a := startAPI(t)
	author := a.login(t, "worker-1")
	other := a.login(t, "worker-2")
	rootID := uuid.NewString()
	a.save(t, author, rootID, normal(rootID, 0))
	a.submit(t, other, uuid.NewString(), normal(rootID, 1)).expect(t, http.StatusForbidden)
	expectRows(t, rootID, 1)
}

func TestInvalidSubmission(t *testing.T) {
	a := startAPI(t)
	cookie := a.login(t, "worker-1")
	id := uuid.NewString()
	request := normal(id, 0)
	for _, tc := range []struct {
		name string
		id   string
		body string
	}{
		{"BrokenJSON", id, "{"},
		{"InvalidID", "invalid", request},
		{"InvalidRootID", id, normal("invalid", 0)},
		{"MismatchedRoot", id, normal(uuid.NewString(), 0)},
		{"CorrectionPointsToItself", id, normal(id, 1)},
		{"NegativeRevision", id, normal(id, -1)},
		{"MissingRevision", id, strings.Replace(request, `"revision":0,`, "", 1)},
		{"MissingTimestamp", id, strings.TrimSuffix(request, `,"inspectedAt":1764130000000}`) + "}"},
		{"FractionalTimestamp", id, strings.Replace(request, "1764130000000", "1764130000000.5", 1)},
		{"OutOfRangeTimestamp", id, strings.Replace(request, "1764130000000", "9223372036854775807", 1)},
		{"InvalidResult", id, payload(id, 0, "P-12", "unknown", "")},
		{"AbnormalWithoutMemo", id, payload(id, 0, "P-12", "abnormal", " \t\n　")},
		{"UnknownEquipment", id, payload(id, 0, "missing", "normal", "")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.submit(t, cookie, tc.id, tc.body).expect(t, http.StatusUnprocessableEntity)
		})
	}
	expectRows(t, id, 0)
}

func TestCorrectionReplay(t *testing.T) {
	a := startAPI(t)
	cookie := a.login(t, "worker-1")
	rootID, correctionID, latestID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	original := normal(rootID, 0)
	first := a.save(t, cookie, rootID, original)
	correction := payload(rootID, 1, "P-12", "abnormal", "누수")
	second := a.save(t, cookie, correctionID, correction)
	latest := a.save(t, cookie, latestID, normal(rootID, 2))
	if !second.ReceivedAt.After(first.ReceivedAt) || !latest.ReceivedAt.After(second.ReceivedAt) {
		t.Fatalf("revision 순서와 접수 시각 순서 불일치: %+v %+v %+v", first, second, latest)
	}

	events, unsubscribe := a.hub.Subscribe()
	t.Cleanup(unsubscribe)
	replay := a.submit(t, cookie, correctionID, correction)
	replay.expect(t, http.StatusOK)
	if got := replay.receipt(t, correctionID); got != second {
		t.Fatalf("정정 재전송으로 접수 응답이 바뀌었다: %+v, 기대 %+v", got, second)
	}
	rootReplay := a.submit(t, cookie, rootID, original)
	rootReplay.expect(t, http.StatusOK)
	if got := rootReplay.receipt(t, rootID); got != first {
		t.Fatalf("원본 접수 응답이 바뀌었다: %+v, 기대 %+v", got, first)
	}
	a.submit(t, cookie, correctionID, payload(uuid.NewString(), 1, "P-12", "abnormal", "누수")).conflict(t, "content_mismatch")
	a.submit(t, cookie, correctionID, payload(rootID, 2, "P-12", "abnormal", "누수")).conflict(t, "content_mismatch")
	expectRows(t, rootID, 3)
	select {
	case event := <-events:
		t.Fatalf("원본 또는 정정 재전송에서 추가 접수 알림 발생: %+v", event)
	default:
	}
}

func TestNormalizedMemoReplay(t *testing.T) {
	a := startAPI(t)
	cookie := a.login(t, "worker-1")
	id := uuid.NewString()
	first := a.save(t, cookie, id, payload(id, 0, "P-12", "abnormal", "가동"))
	for _, tc := range []struct {
		name string
		memo string
	}{
		{"NFD", "\u1100\u1161\u1103\u1169\u11bc"},
		{"Whitespace", "\r\n 가동　\t"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := a.submit(t, cookie, id, payload(id, 0, "P-12", "abnormal", tc.memo))
			res.expect(t, http.StatusOK)
			if got := res.receipt(t, id); got != first {
				t.Fatalf("정규화한 재전송 응답이 바뀌었다: %+v, 기대 %+v", got, first)
			}
		})
	}
	expectRows(t, id, 1)
}

type putRequest struct {
	cookie   *http.Cookie
	id, body string
}

func (a api) submitConcurrently(t *testing.T, requests []putRequest) []response {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	type result struct {
		index int
		response
		err error
	}
	ready := make(chan struct{}, len(requests))
	start := make(chan struct{})
	done := make(chan result, len(requests))
	for index, request := range requests {
		go func() {
			ready <- struct{}{}
			<-start
			res, err := a.put(ctx, request.cookie, request.id, request.body)
			done <- result{index: index, response: res, err: err}
		}()
	}
	for range requests {
		<-ready
	}
	close(start)

	responses := make([]response, len(requests))
	for range requests {
		result := <-done
		if result.err != nil {
			t.Error(result.err)
		}
		responses[result.index] = result.response
	}
	if t.Failed() {
		t.FailNow()
	}
	return responses
}

func TestConcurrentIdenticalPuts(t *testing.T) {
	a := startAPI(t)
	cookie := a.login(t, "worker-1")
	supervisor := a.login(t, "supervisor")
	stream := a.openBoardStream(t, supervisor)
	if frame := readSSEFrame(t, stream); frame != ":\n" {
		t.Fatalf("첫 SSE 프레임은 하트비트여야 한다: %q", frame)
	}
	id := uuid.NewString()
	request := putRequest{cookie: cookie, id: id, body: normal(id, 0)}
	requests := make([]putRequest, 50)
	for i := range requests {
		requests[i] = request
	}
	responses := a.submitConcurrently(t, requests)

	var created, replayed int
	for _, res := range responses {
		switch res.status {
		case http.StatusCreated:
			created++
		case http.StatusOK:
			replayed++
		default:
			t.Fatalf("동시 요청에서 HTTP %d 수신, 본문 %s", res.status, res.body)
		}
		if !bytes.Equal(res.body, responses[0].body) {
			t.Fatalf("동시 요청의 접수 응답 불일치: %s, %s", res.body, responses[0].body)
		}
		res.receipt(t, id)
	}
	if created != 1 || replayed != 49 {
		t.Fatalf("동시 요청 응답 HTTP 201 %d건, 200 %d건", created, replayed)
	}
	expectRows(t, id, 1)
	want := pumpRow(id, 0, "normal", "", responses[0].receipt(t, id).ReceivedAt)
	a.expectStreamRow(t, supervisor, stream, want)

	memo := "진동\n재점검"
	correction := a.save(t, cookie, uuid.NewString(), payload(id, 1, "P-12", "abnormal", memo))
	want = pumpRow(id, 1, "abnormal", memo, correction.ReceivedAt)
	a.expectStreamRow(t, supervisor, stream, want)

	// 같은 스트림에서 다음 원본을 받으면 그 전에 발행된 알림을 모두 확인한 셈이다.
	nextID := uuid.NewString()
	next := a.save(t, cookie, nextID, normal(nextID, 0))
	a.expectStreamRow(t, supervisor, stream, pumpRow(nextID, 0, "normal", "", next.ReceivedAt))
}

func TestConcurrentCorrections(t *testing.T) {
	a := startAPI(t)
	firstDevice := a.login(t, "worker-1")
	secondDevice := a.login(t, "worker-1")
	rootID := uuid.NewString()
	a.save(t, firstDevice, rootID, normal(rootID, 0))
	events, unsubscribe := a.hub.Subscribe()
	t.Cleanup(unsubscribe)
	requests := []putRequest{
		{cookie: firstDevice, id: uuid.NewString(), body: payload(rootID, 1, "P-12", "abnormal", "누수")},
		{cookie: secondDevice, id: uuid.NewString(), body: payload(rootID, 1, "P-12", "abnormal", "진동")},
	}
	var winner receipt
	var winnerMemo string
	var conflicts int
	for i, res := range a.submitConcurrently(t, requests) {
		if res.status == http.StatusCreated {
			winner = res.receipt(t, requests[i].id)
			winnerMemo = []string{"누수", "진동"}[i]
			continue
		}
		res.conflict(t, "revision_taken")
		conflicts++
	}
	if winner.ID == "" || conflicts != 1 {
		t.Fatalf("동시 정정 응답 불일치: 저장된 ID %q, 409 %d건", winner.ID, conflicts)
	}
	expectRows(t, rootID, 2)
	expectOnlyEvent(t, events, pumpRow(rootID, 1, "abnormal", winnerMemo, winner.ReceivedAt))
}
