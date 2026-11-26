package main

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"offline-inspection-board/internal/auth"
)

func TestSessionProfileAndLogout(t *testing.T) {
	a := startAPI(t)
	a.get(t, nil, "/api/me").expect(t, http.StatusUnauthorized)

	cookie := a.login(t, "worker-1")
	profile := a.get(t, cookie, "/api/me")
	profile.expect(t, http.StatusOK)
	var user auth.User
	if err := json.Unmarshal(profile.body, &user); err != nil || user != (auth.User{ID: "worker-1", Name: "직원 1", Role: "worker"}) {
		t.Fatalf("사용자 정보 불일치: %s (%v)", profile.body, err)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, a.URL+"/api/session/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	res, err := a.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("로그아웃 HTTP %d", res.StatusCode)
	}
	cookies := res.Cookies()
	if len(cookies) != 1 || cookies[0].Name != cookie.Name || cookies[0].Path != cookie.Path || cookies[0].Value != "" || cookies[0].MaxAge >= 0 {
		t.Fatalf("세션 만료 쿠키 불일치: %v", cookies)
	}
	a.get(t, cookie, "/api/me").expect(t, http.StatusUnauthorized)
	var exists bool
	if err := testPool.QueryRow(t.Context(), "SELECT EXISTS (SELECT 1 FROM sessions WHERE token = $1)", cookie.Value).Scan(&exists); err != nil || exists {
		t.Fatalf("세션 삭제 실패: %v (%v)", exists, err)
	}
}

func TestActiveEquipmentForAnyLoggedInRole(t *testing.T) {
	a := startAPI(t)
	a.get(t, nil, "/api/equipment").expect(t, http.StatusUnauthorized)
	want := []equipment{{"F-08", "배기 팬"}, {"P-12", "냉각수 펌프"}, {"V-04", "급수 밸브"}}
	for _, userID := range []string{"worker-1", "supervisor"} {
		cookie := a.login(t, userID)
		res := a.get(t, cookie, "/api/equipment")
		res.expect(t, http.StatusOK)
		var items []equipment
		if err := json.Unmarshal(res.body, &items); err != nil || !slices.Equal(items, want) {
			t.Fatalf("%s의 설비 목록 불일치: %s (%v)", userID, res.body, err)
		}
	}
}
