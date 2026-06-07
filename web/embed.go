// Package web embeds the built Svelte SPA so weft ships as a single binary.
// The Vite build writes its output to web/dist (see web/vite.config.js); the
// Makefile's `web` target produces it before `go build`.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets returns the SPA file tree rooted at the directory containing
// index.html. If the frontend has not been built, the returned FS contains only
// the placeholder index.html committed to the repo.
func Assets() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
