package srv

// Serves the React UI (web/dist, embedded at build time) for every path the
// route table does not claim: real files as-is, everything else index.html
// so client-side routes (/apps/x, /settings/…) survive a reload.

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"srv.exe.dev/web"
)

const uiNotBuilt = `<!doctype html><meta charset="utf-8"><title>release-hub</title>
<body style="font:14px system-ui;margin:4rem auto;max-width:36rem;line-height:1.5">
<h1 style="font-size:18px">The UI is not built into this binary</h1>
<p>Run <code>make build</code> (builds <code>web/</code>, then the Go binary), or
<code>make dev</code> for the Vite dev server. The API and device endpoints work regardless.</p>`

func spaHandler(dist fs.FS) http.Handler {
	if dist == nil {
		dist = web.Dist()
	}
	index, indexErr := fs.ReadFile(dist, "index.html")
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if st, err := fs.Stat(dist, name); err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					// Vite content-hashes everything under assets/.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				files.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(name, "assets/") {
				// A stale tab asking for an old chunk: 404, not the HTML shell.
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if indexErr != nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(uiNotBuilt))
			return
		}
		_, _ = w.Write(index)
	})
}
