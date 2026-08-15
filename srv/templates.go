package srv

// HTML template loading and rendering. Templates come from the on-disk dir
// when present (dev checkout) or the go:embed FS baked into the binary.

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed templates/*.html
var embeddedTemplatesFS embed.FS

//go:embed static
var embeddedStaticFS embed.FS

// staticHandler serves the embedded CSS/JS (baked into the binary; changing
// them means a new binary).
func staticHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /static/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		b, _ := embeddedStaticFS.ReadFile("static/style.css")
		w.Write(b)
	})
	mux.HandleFunc("GET /static/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		b, _ := embeddedStaticFS.ReadFile("static/app.js")
		w.Write(b)
	})
	mux.HandleFunc("GET /static/Inter-Regular.woff2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "font/woff2")
		b, _ := embeddedStaticFS.ReadFile("static/Inter-Regular.woff2")
		w.Write(b)
	})
	mux.HandleFunc("GET /static/Inter-Bold.woff2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "font/woff2")
		b, _ := embeddedStaticFS.ReadFile("static/Inter-Bold.woff2")
		w.Write(b)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	return mux
}

// parsePageTemplate parses base + page template. Pages define a "content"
// block and expect a {{template "base.html" .}} header line.
func parsePageTemplate(dir, name string) (*template.Template, error) {
	var tmpl *template.Template
	var err error
	if _, statErr := os.Stat(filepath.Join(dir, name)); statErr == nil {
		tmpl, err = template.ParseFiles(filepath.Join(dir, name), filepath.Join(dir, "base.html"))
	} else {
		tmpl, err = template.ParseFS(embeddedTemplatesFS, "templates/"+name, "templates/base.html")
	}
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", name, err)
	}
	return tmpl, nil
}

// flash reads and clears the short-lived rh_flash cookie.
func flash(r *http.Request) string {
	c, err := r.Cookie("rh_flash")
	if err != nil {
		return ""
	}
	return c.Value
}

func withFlash(r *http.Request, msg string) context.Context {
	return context.WithValue(r.Context(), flashKey{}, msg)
}

type flashKey struct{}

func flashFrom(r *http.Request) string {
	v, _ := r.Context().Value(flashKey{}).(string)
	return v
}
