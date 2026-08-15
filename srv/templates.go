package srv

// HTML template loading and rendering. Templates come from the on-disk dir
// when present (dev checkout) or the go:embed FS baked into the binary.

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed templates/*.html
var embeddedTemplatesFS embed.FS

// assetVersion fingerprints the embedded static assets so URLs change with
// every build. Behind CDN caches (Cloudflare & co.) this guarantees a new
// binary can never serve stale CSS/JS: the new HTML references new URLs.
var assetVersion = func() string {
	h := sha256.New()
	fs.WalkDir(embeddedStaticFS, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := embeddedStaticFS.ReadFile(p)
		if err != nil {
			return err
		}
		h.Write([]byte(p))
		h.Write(b)
		return nil
	})
	return fmt.Sprintf("%x", h.Sum(nil))[:10]
}()

//go:embed static
var embeddedStaticFS embed.FS

// staticHandler serves the embedded CSS/JS (baked into the binary; changing
// them means a new binary).
func staticHandler() http.Handler {
	mux := http.NewServeMux()
	// Versioned, content-hashed asset URLs: immutable + cached hard by any CDN.
	mux.HandleFunc("GET /static/{v}/{file}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("v") != assetVersion {
			http.NotFound(w, r)
			return
		}
		file := "static/" + filepath.Base(r.PathValue("file"))
		b, err := embeddedStaticFS.ReadFile(file)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		switch {
		case strings.HasSuffix(file, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(file, ".js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		case strings.HasSuffix(file, ".woff2"):
			w.Header().Set("Content-Type", "font/woff2")
		}
		w.Write(b)
	})
	// Unversioned fallback for anything still holding an old URL (CDN edge
	// caches may keep serving it for hours): serve current assets, no-cache.
	mux.HandleFunc("GET /static/", func(w http.ResponseWriter, r *http.Request) {
		file := "static/" + filepath.Base(r.URL.Path)
		b, err := embeddedStaticFS.ReadFile(file)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		switch {
		case strings.HasSuffix(file, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(file, ".js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		case strings.HasSuffix(file, ".woff2"):
			w.Header().Set("Content-Type", "font/woff2")
		}
		w.Write(b)
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
