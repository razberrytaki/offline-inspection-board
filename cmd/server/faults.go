package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const faultLogSize = 20

type faultAttempt struct {
	At      time.Time `json:"at"`
	ID      string    `json:"id"`
	Outcome any       `json:"outcome"` // HTTP 상태 코드, dropped 또는 offline
}

type inspectionFaults struct {
	pool *pgxpool.Pool

	mu        sync.Mutex
	remaining int64
	offline   bool
	online    context.Context
	cancel    context.CancelFunc
	log       [faultLogSize]faultAttempt
	next      int
	size      int
}

func faultsEnabled() bool {
	return os.Getenv("FAULT_INJECTION") == "1"
}

func withFaults(mux *http.ServeMux, pool *pgxpool.Pool) http.Handler {
	if !faultsEnabled() {
		return mux
	}
	online, cancel := context.WithCancel(context.Background())
	f := &inspectionFaults{pool: pool, online: online, cancel: cancel}
	mux.HandleFunc("GET /api/dev/faults", f.state)
	mux.HandleFunc("POST /api/dev/faults", f.configure)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/api/dev/") {
			mux.ServeHTTP(w, r)
			return
		}
		_, pattern := mux.Handler(r)
		isPut := pattern == "PUT /api/inspections/{id}"
		f.mu.Lock()
		offline, online := f.offline, f.online
		if offline && isPut {
			f.record(strings.TrimPrefix(r.URL.Path, "/api/inspections/"), "offline")
		}
		f.mu.Unlock()
		if offline {
			cutConnection(w)
			return
		}
		switch {
		case isPut:
			f.put(mux, w, r)
		case pattern == "GET /api/board/events":
			ctx, cancel := context.WithCancel(r.Context())
			defer cancel()
			stop := context.AfterFunc(online, cancel)
			defer stop()
			mux.ServeHTTP(w, r.WithContext(ctx))
			if online.Err() != nil {
				panic(http.ErrAbortHandler)
			}
		default:
			mux.ServeHTTP(w, r)
		}
	})
}

func (f *inspectionFaults) configure(w http.ResponseWriter, r *http.Request) {
	var settings *struct {
		DropAfterCommit json.RawMessage `json:"dropAfterCommit"`
		Offline         json.RawMessage `json:"offline"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil || settings == nil ||
		decoder.Decode(new(json.RawMessage)) != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var remaining *int64
	if settings.DropAfterCommit != nil {
		if err := json.Unmarshal(settings.DropAfterCommit, &remaining); err != nil || remaining == nil || *remaining < 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	var offline *bool
	if settings.Offline != nil {
		if err := json.Unmarshal(settings.Offline, &offline); err != nil || offline == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	f.mu.Lock()
	if remaining != nil {
		f.remaining = *remaining
	}
	if offline != nil && f.offline != *offline {
		f.offline = *offline
		if f.offline {
			f.cancel()
		} else {
			f.online, f.cancel = context.WithCancel(context.Background())
		}
	}
	f.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (f *inspectionFaults) state(w http.ResponseWriter, r *http.Request) {
	var state struct {
		DropAfterCommit int64          `json:"dropAfterCommit"`
		Offline         bool           `json:"offline"`
		InspectionCount int64          `json:"inspectionCount"`
		Log             []faultAttempt `json:"log"`
	}
	f.mu.Lock()
	state.DropAfterCommit = f.remaining
	state.Offline = f.offline
	state.Log = make([]faultAttempt, f.size)
	for i := range state.Log {
		state.Log[i] = f.log[(f.next+faultLogSize-f.size+i)%faultLogSize]
	}
	f.mu.Unlock()

	if err := f.pool.QueryRow(r.Context(), "SELECT count(*) FROM inspections").Scan(&state.InspectionCount); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

func (f *inspectionFaults) put(next http.Handler, w http.ResponseWriter, r *http.Request) {
	response := httptest.NewRecorder()
	next.ServeHTTP(response, r)

	f.mu.Lock()
	drop := response.Code == http.StatusCreated && f.remaining > 0
	if drop {
		f.remaining--
		f.record(r.PathValue("id"), "dropped")
	} else {
		f.record(r.PathValue("id"), response.Code)
	}
	f.mu.Unlock()

	if drop {
		cutConnection(w)
		return
	}
	for name, values := range response.Header() {
		w.Header()[name] = values
	}
	w.WriteHeader(response.Code)
	_, _ = w.Write(response.Body.Bytes())
}

// record는 mu를 잡은 상태에서 호출한다.
func (f *inspectionFaults) record(id string, outcome any) {
	f.log[f.next] = faultAttempt{At: time.Now().UTC(), ID: id, Outcome: outcome}
	f.next = (f.next + 1) % faultLogSize
	f.size = min(f.size+1, faultLogSize)
}

func cutConnection(w http.ResponseWriter) {
	conn, _, err := http.NewResponseController(w).Hijack()
	if err != nil {
		panic(http.ErrAbortHandler)
	}
	_ = conn.Close()
}
