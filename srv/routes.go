package srv

import "net/http"

// routes is the single canonical route table, used by Serve and by tests.
// Every handler lives in handlers_*.go; registration happens only here.
func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()

	// UI (session auth; setup/login always reachable)
	mux.HandleFunc("GET /setup", s.handleFirstRun)
	mux.HandleFunc("POST /setup", s.handleFirstRun)
	mux.HandleFunc("GET /login", s.handleLogin)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("GET /logout", s.handleLogout)
	mux.HandleFunc("GET /{$}", s.requireUI(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(withFlash(r, flash(r)))
		s.handleHome(w, r)
	}))
	mux.HandleFunc("POST /apps", s.requireUI(s.handleCreateAppUI))
	mux.HandleFunc("POST /apps/{slug}/platforms", s.requireUI(s.handleAddPlatformUI))
	mux.HandleFunc("POST /tokens", s.requireUI(s.handleCreateTokenUI))
	mux.HandleFunc("GET /apps/{slug}", s.requireUI(s.handleAppDetail))
	mux.HandleFunc("GET /settings", s.requireUI(s.handleSettings))
	mux.HandleFunc("POST /settings", s.requireUI(s.handleSettings))

	// API (bearer auth)
	mux.HandleFunc("GET /api/apps", s.requireAPI(s.handleApiListApps))
	mux.HandleFunc("POST /api/apps", s.requireAPI(s.handleApiCreateApp))
	mux.HandleFunc("POST /api/tokens", s.requireAPI(s.handleApiCreateToken))
	mux.HandleFunc("POST /api/apps/{slug}/platforms", s.requireAPI(s.handleApiAddPlatform))
	mux.HandleFunc("POST /api/apps/{slug}/releases", s.requireAPI(s.handleApiUpload))
	mux.HandleFunc("POST /api/apps/{slug}/{platform}/releases", s.requireAPI(s.handleApiUpload))
	mux.HandleFunc("GET /api/apps/{slug}/releases", s.requireAPI(s.handleApiReleases))
	mux.HandleFunc("GET /api/apps/{slug}/{platform}/releases", s.requireAPI(s.handleApiReleases))
	mux.HandleFunc("POST /api/apps/{slug}/play", s.requireAPI(s.handleApiSetPlay))
	mux.HandleFunc("POST /api/apps/{slug}/{platform}/play", s.requireAPI(s.handleApiSetPlay))
	mux.HandleFunc("POST /api/apps/{slug}/signing", s.requireAPI(s.handleApiSetSigning))
	mux.HandleFunc("POST /api/apps/{slug}/{platform}/signing", s.requireAPI(s.handleApiSetSigning))
	mux.HandleFunc("GET /api/apps/{slug}/signing", s.requireAPI(s.handleApiGetSigning))
	mux.HandleFunc("GET /api/apps/{slug}/{platform}/signing", s.requireAPI(s.handleApiGetSigning))
	mux.HandleFunc("POST /api/apps/{slug}/signing/delete", s.requireAPI(s.handleApiDeleteSigning))
	mux.HandleFunc("POST /api/apps/{slug}/{platform}/signing/delete", s.requireAPI(s.handleApiDeleteSigning))

	// Public (devices): manifest + artifact download need no auth.
	mux.HandleFunc("GET /api/apps/{slug}/manifest", s.handleManifest)
	mux.HandleFunc("GET /api/apps/{slug}/{platform}/manifest", s.handleManifest)
	mux.HandleFunc("GET /artifacts/{slug}/{platform}/{file}", s.handleArtifact)
	mux.HandleFunc("GET /artifacts/{slug}/{file}", s.handleArtifact) // legacy path shape

	mux.Handle("GET /static/", staticHandler())

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	return mux
}
