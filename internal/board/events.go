package board

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

func (h Handler) Notify(ctx context.Context, rootID string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	records, err := readRevisions(ctx, h.Pool, revisionsForRoot+" ORDER BY i.revision DESC LIMIT 1", rootID)
	if err == nil && len(records) == 0 {
		err = pgx.ErrNoRows
	}
	if err != nil {
		slog.Error("보드 알림 조회 실패", "rootId", rootID, "error", err)
		return
	}
	h.Hub.Publish(records[0].Row)
}

func (h Handler) Events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method == http.MethodHead {
		return
	}

	rows, unsubscribe := h.Hub.Subscribe()
	defer unsubscribe()
	controller := http.NewResponseController(w)
	flush := func(frame string) bool {
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
		_, err := io.WriteString(w, frame)
		return err == nil && controller.Flush() == nil
	}
	if !flush("") {
		return
	}

	heartbeat := time.NewTicker(h.HeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if !flush(":\n\n") {
				return
			}
		case row, ok := <-rows:
			if !ok {
				return
			}
			data, err := json.Marshal(row)
			if err != nil {
				slog.Error("보드 알림 변환 실패", "rootId", row.RootID, "error", err)
				return
			}
			if !flush("event: row\ndata: " + string(data) + "\n\n") {
				return
			}
		}
	}
}
