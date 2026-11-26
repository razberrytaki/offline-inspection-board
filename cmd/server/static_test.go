package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"offline-inspection-board/internal/board"
)

func TestStaticFilesAndSPAFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "_app"), 0755); err != nil {
		t.Fatal(err)
	}
	const shell = "<!doctype html><title>현장 점검</title>"
	const script = "export const available = true\n"
	for name, content := range map[string]string{spaFallback: shell, "_app/app.js": script} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	handler := appHandler(routes(testPool, &board.Hub{}, time.Second), staticDirectory(dir))
	for _, tc := range []struct {
		path, content string
		status        int
	}{
		{"/_app/app.js", script, http.StatusOK},
		{"/", shell, http.StatusOK},
		{"/board", shell, http.StatusOK},
		{"/board/detail", shell, http.StatusOK},
		{"/api", "", http.StatusNotFound},
		{"/api/missing", "", http.StatusNotFound},
		{"/api/board", "", http.StatusUnauthorized},
	} {
		t.Run(tc.path, func(t *testing.T) {
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if res.Code != tc.status || (tc.content != "" && res.Body.String() != tc.content) ||
				(tc.content == "" && strings.Contains(res.Body.String(), shell)) {
				t.Fatalf("%s: HTTP %d, 본문 %q", tc.path, res.Code, res.Body.String())
			}
		})
	}
}

func TestAppWithoutFrontendBuild(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "build")
	handler := appHandler(routes(testPool, &board.Hub{}, time.Second), staticDirectory(dir))
	for path, want := range map[string]int{"/": http.StatusNotFound, "/api/me": http.StatusUnauthorized} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != want {
			t.Fatalf("%s: HTTP %d, 기대 %d", path, res.Code, want)
		}
	}
}
