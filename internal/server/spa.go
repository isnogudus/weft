package server

import (
	"io/fs"
	"net/http"
	"strings"
)

// spaHandler serves the embedded single-page app: static files when they exist,
// otherwise index.html so client-side routes resolve. API paths never reach
// here (they are mounted under /api).
func spaHandler(static fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" || p == "index.html" {
			serveIndex(w, static)
			return
		}
		if _, err := fs.Stat(static, p); err != nil {
			serveIndex(w, static) // unknown path -> SPA entry point
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, static fs.FS) {
	b, err := fs.ReadFile(static, "index.html")
	if err != nil {
		http.Error(w, "frontend not built", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}
