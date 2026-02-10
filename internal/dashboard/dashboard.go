package dashboard

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

//go:embed templates/*.html templates/partials/*.html
var templateFiles embed.FS

// Templates holds parsed HTML templates for the dashboard.
var Templates *template.Template

func init() {
	sub, _ := fs.Sub(templateFiles, "templates")
	Templates = template.Must(template.ParseFS(sub, "*.html", "partials/*.html"))
}

// Handler returns an http.Handler that serves the embedded dashboard files.
// The apiBase parameter is injected so the JS knows where the API lives.
func Handler() http.Handler {
	sub, _ := fs.Sub(staticFiles, "static")
	return http.FileServer(http.FS(sub))
}
