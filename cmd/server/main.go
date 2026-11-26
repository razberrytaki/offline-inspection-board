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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	pool, err := db.Open(ctx, databaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	server := &http.Server{
		Addr:              addr,
		Handler:           routes(pool, &board.Hub{}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	slog.Info("서버 시작", "addr", addr)
	return server.ListenAndServe()
}

func routes(pool *pgxpool.Pool, hub *board.Hub) http.Handler {
	sessions := auth.Sessions{Pool: pool}
	inspections := inspection.Handler{Store: inspection.Store{Pool: pool}, Hub: hub}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /session", sessions.Login)
	mux.Handle("PUT /inspections/{id}", sessions.Require(http.HandlerFunc(inspections.Put)))
	return mux
}
