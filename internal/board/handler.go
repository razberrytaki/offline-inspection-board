package board

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	Pool              *pgxpool.Pool
	Hub               *Hub
	HeartbeatInterval time.Duration
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	abnormalOnly := false
	if filters, ok := query["filter"]; ok {
		if len(filters) != 1 || (filters[0] != "all" && filters[0] != "abnormal") {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		abnormalOnly = filters[0] == "abnormal"
	}

	records, err := readRevisions(r.Context(), h.Pool, listQuery, abnormalOnly)
	if err != nil {
		boardError(w, err)
		return
	}
	rows := make([]Row, 0, len(records))
	for _, record := range records {
		rows = append(rows, record.Row)
	}
	writeJSON(w, rows)
}

func (h Handler) Detail(w http.ResponseWriter, r *http.Request) {
	rootID, err := uuid.Parse(r.PathValue("rootId"))
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	history, err := readRevisions(r.Context(), h.Pool, revisionsForRoot+" ORDER BY i.revision", rootID.String())
	if err != nil {
		boardError(w, err)
		return
	}
	if len(history) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, struct {
		Current Row        `json:"current"`
		History []revision `json:"history"`
	}{history[len(history)-1].Row, history})
}

func boardError(w http.ResponseWriter, err error) {
	slog.Error("보드 조회 실패", "error", err)
	w.WriteHeader(http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}
