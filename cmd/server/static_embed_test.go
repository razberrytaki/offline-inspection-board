//go:build embed

package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestEmbeddedBinary(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "oib")
	build := exec.CommandContext(t.Context(), "go", "build", "-tags", "embed", "-o", bin, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("바이너리 빌드: %v\n%s", err, output)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = dir
	cmd.Env = slices.DeleteFunc(cmd.Environ(), func(value string) bool {
		return strings.HasPrefix(value, "STATIC_DIR=") || strings.HasPrefix(value, "FAULT_INJECTION=")
	})
	cmd.Env = append(cmd.Env, "ADDR="+addr, "DATABASE_URL="+testPool.Config().ConnString())
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
		if t.Failed() {
			t.Log(logs.String())
		}
	})

	base := "http://" + addr
	ctx, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	defer cancelStartup()
	read := func(path string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			return nil, err
		}
		return http.DefaultClient.Do(req)
	}
	poll := time.NewTicker(50 * time.Millisecond)
	defer poll.Stop()
	for {
		res, err := read("/")
		if err == nil {
			res.Body.Close()
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("바이너리 시작 실패: %v", err)
		case <-poll.C:
		}
	}

	for _, tc := range []struct {
		path, asset string
		status      int
	}{
		{"/", "200.html", http.StatusOK},
		{"/board", "200.html", http.StatusOK},
		{"/_app/version.json", "_app/version.json", http.StatusOK},
		{"/service-worker.js", "service-worker.js", http.StatusOK},
		{"/api/me", "", http.StatusUnauthorized},
	} {
		t.Run(tc.path, func(t *testing.T) {
			res, err := read(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			response{status: res.StatusCode}.expect(t, tc.status)
			if tc.asset == "" {
				return
			}
			got, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("..", "..", "web", "build", tc.asset))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%s 응답과 빌드 파일이 다르다", tc.path)
			}
		})
	}
}
