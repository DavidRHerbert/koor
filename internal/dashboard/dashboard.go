package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

// Handler returns an http.Handler that serves the embedded dashboard files.
// The apiBase parameter is injected so the JS knows where the API lives.
func Handler() http.Handler {
	sub, _ := fs.Sub(staticFiles, "static")
	return http.FileServer(http.FS(sub))
}
