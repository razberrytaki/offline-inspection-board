package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
)

type inspectionFaults struct {
	remaining atomic.Int64
}

func faultsEnabled() bool {
	return os.Getenv("FAULT_INJECTION") == "1"
}

func (f *inspectionFaults) configure(w http.ResponseWriter, r *http.Request) {
	var settings struct {
		DropAfterCommit *int64 `json:"dropAfterCommit"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	if err := decoder.Decode(&settings); err != nil || settings.DropAfterCommit == nil ||
		*settings.DropAfterCommit < 0 || decoder.Decode(new(json.RawMessage)) != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.remaining.Store(*settings.DropAfterCommit)
	w.WriteHeader(http.StatusNoContent)
}

func (f *inspectionFaults) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for {
			remaining := f.remaining.Load()
			if remaining == 0 {
				next.ServeHTTP(w, r)
				return
			}
			if f.remaining.CompareAndSwap(remaining, remaining-1) {
				break
			}
		}

		next.ServeHTTP(httptest.NewRecorder(), r)
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			panic(http.ErrAbortHandler)
		}
		_ = conn.Close()
	})
}
