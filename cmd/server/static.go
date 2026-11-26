package main

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

const spaFallback = "200.html"

func appHandler(api http.Handler, files fs.FS) http.Handler {
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
		if !strings.HasSuffix(r.URL.Path, "/") && serveAsset(w, r, files, name) {
			return
		}
		if !serveAsset(w, r, files, spaFallback) {
			http.NotFound(w, r)
		}
	})
	return mux
}

type staticDirectory string

func (dir staticDirectory) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return os.OpenInRoot(string(dir), name)
}

func serveAsset(w http.ResponseWriter, r *http.Request, files fs.FS, name string) bool {
	file, err := files.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	content, seekable := file.(io.ReadSeeker)
	info, err := file.Stat()
	if err != nil || !seekable || !info.Mode().IsRegular() {
		return false
	}
	if name == spaFallback {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, path.Base(name), info.ModTime(), content)
	return true
}
