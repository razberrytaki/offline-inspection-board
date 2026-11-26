package main

import (
	"net/http"
	"os"
	"path"
	"strings"
)

const spaFallback = "200.html"

func appHandler(api http.Handler, staticDir string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api", api)
	mux.Handle("/api/", api)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if !strings.HasSuffix(r.URL.Path, "/") && serveAsset(w, r, staticDir, name) {
			return
		}
		if !serveAsset(w, r, staticDir, spaFallback) {
			http.NotFound(w, r)
		}
	})
	return mux
}

func serveAsset(w http.ResponseWriter, r *http.Request, dir, name string) bool {
	file, err := os.OpenInRoot(dir, name)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if name == spaFallback {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, path.Base(name), info.ModTime(), file)
	return true
}
