package main

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type equipment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func listEquipment(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := pool.Query(r.Context(), "SELECT id, name FROM equipment WHERE NOT retired ORDER BY id")
		if err == nil {
			var items []equipment
			items, err = pgx.CollectRows(rows, pgx.RowToStructByPos[equipment])
			if err == nil {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				json.NewEncoder(w).Encode(items)
				return
			}
		}
		slog.Error("설비 목록 조회 실패", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}
