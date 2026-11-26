package inspection

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"offline-inspection-board/internal/auth"
	"offline-inspection-board/internal/board"
)

type Handler struct {
	Store Store
	Hub   *board.Hub
}

var outcomeResponses = map[Outcome]struct {
	status int
	reason string
}{
	Created:         {http.StatusCreated, ""},
	Replayed:        {http.StatusOK, ""},
	ContentMismatch: {http.StatusConflict, "content_mismatch"},
	RevisionTaken:   {http.StatusConflict, "revision_taken"},
	MissingPrevious: {http.StatusConflict, "missing_previous"},
	Forbidden:       {http.StatusForbidden, ""},
}

func (h Handler) Put(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	submission, err := Parse(r.PathValue("id"), body, auth.UserID(r.Context()))
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	outcome, record, err := h.Store.Put(r.Context(), submission)
	if errors.Is(err, ErrUnknownEquipment) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	if err != nil {
		slog.Error("점검 저장 실패", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if outcome == Created {
		h.Hub.Publish(board.Event{ID: record.ID, RootID: record.RootID, Revision: record.Revision})
	}

	response := outcomeResponses[outcome]
	switch response.status {
	case http.StatusCreated, http.StatusOK:
		writeJSON(w, response.status, struct {
			ID         string    `json:"id"`
			ReceivedAt time.Time `json:"receivedAt"`
		}{record.ID, record.ReceivedAt})
	case http.StatusConflict:
		var current *Record
		if outcome == ContentMismatch && record.AuthorID == submission.AuthorID {
			current = &record
		}
		writeJSON(w, response.status, struct {
			Reason  string  `json:"reason"`
			Current *Record `json:"current,omitempty"`
		}{response.reason, current})
	default:
		w.WriteHeader(response.status)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
