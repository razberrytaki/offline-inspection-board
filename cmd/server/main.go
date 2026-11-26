package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"offline-inspection-board/internal/auth"
	"offline-inspection-board/internal/board"
	"offline-inspection-board/internal/db"
	"offline-inspection-board/internal/inspection"
)

func main() {
	if err := run(); err != nil {
		slog.Error("서버 실행 실패", "error", err)
		os.Exit(1)
	}
}

func run() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL 환경 변수를 설정하세요")
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	files, err := staticFiles()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	pool, err := db.Open(ctx, databaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	server := &http.Server{
		Addr:              addr,
		Handler:           appHandler(routes(pool, &board.Hub{}, 15*time.Second), files),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if faultsEnabled() {
		// 브라우저가 재사용한 연결의 오류를 자체 재시도로 숨기지 않게 한다.
		server.SetKeepAlivesEnabled(false)
	}
	slog.Info("서버 시작", "addr", addr)
	return server.ListenAndServe()
}

func routes(pool *pgxpool.Pool, hub *board.Hub, heartbeatInterval time.Duration) http.Handler {
	sessions := auth.Sessions{Pool: pool}
	boards := board.Handler{Pool: pool, Hub: hub, HeartbeatInterval: heartbeatInterval}
	inspections := inspection.Handler{Store: inspection.Store{Pool: pool}, Board: boards}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/session", sessions.Login)
	mux.HandleFunc("POST /api/session/logout", sessions.Logout)
	mux.Handle("GET /api/me", sessions.Require(http.HandlerFunc(auth.Me)))
	mux.Handle("GET /api/equipment", sessions.Require(listEquipment(pool)))
	mux.Handle("PUT /api/inspections/{id}", sessions.Require(http.HandlerFunc(inspections.Put)))
	mux.Handle("GET /api/board", sessions.RequireRole("supervisor", http.HandlerFunc(boards.List)))
	mux.Handle("GET /api/board/{rootId}", sessions.RequireRole("supervisor", http.HandlerFunc(boards.Detail)))
	mux.Handle("GET /api/board/events", sessions.RequireRole("supervisor", http.HandlerFunc(boards.Events)))
	return withFaults(mux, pool)
}
