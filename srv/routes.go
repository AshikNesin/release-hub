package srv

import "net/http"

// routes is the single canonical route table, used by Serve and by tests.
// Registration happens only here; the handlers live beside their API in
// api_apps.go, api_releases.go, api_oidc.go, api_v1*.go, public.go, signing.go.
func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()

	// UI: the React app (web/, embedded) and its JSON API (/api/v1, session
	// auth). Every path the table does not claim falls through to the SPA.
	s.v1Routes(mux)
	mux.Handle("/", spaHandler(s.uiFS))
	// Old server-rendered pages lived at /login, /setup, /apps/{slug}, /settings;
	// those URLs are now client routes, served by the SPA fallback above.

	// API (bearer auth)
	mux.HandleFunc("POST /api/oidc/exchange", s.handleApiOidcExchange)
	mux.HandleFunc("GET /api/apps", s.requireAPI(s.handleApiListApps))
	mux.HandleFunc("POST /api/apps", s.requireAPI(s.handleApiCreateApp))
	mux.HandleFunc("DELETE /api/apps/{slug}", s.requireAPI(s.handleApiDeleteApp))
	mux.HandleFunc("POST /api/tokens", s.requireAPI(s.handleApiCreateToken))
	mux.HandleFunc("POST /api/apps/{slug}/platforms", s.requireAPI(s.handleApiAddPlatform))
	mux.HandleFunc("POST /api/apps/{slug}/releases", s.requireAPI(s.handleApiUpload))
	mux.HandleFunc("POST /api/apps/{slug}/{platform}/releases", s.requireAPI(s.handleApiUpload))
	mux.HandleFunc("DELETE /api/apps/{slug}/releases/{versionCode}", s.requireAPI(s.handleApiDeleteRelease))
	mux.HandleFunc("DELETE /api/apps/{slug}/{platform}/releases/{versionCode}", s.requireAPI(s.handleApiDeleteRelease))
	mux.HandleFunc("GET /api/apps/{slug}/releases", s.requireAPI(s.handleApiReleases))
	mux.HandleFunc("GET /api/apps/{slug}/{platform}/releases", s.requireAPI(s.handleApiReleases))
	mux.HandleFunc("POST /api/apps/{slug}/play", s.requireAPI(s.handleApiSetPlay))
	mux.HandleFunc("POST /api/apps/{slug}/{platform}/play", s.requireAPI(s.handleApiSetPlay))
	mux.HandleFunc("POST /api/apps/{slug}/testers", s.requireAPI(s.handleApiInviteTesters))
	mux.HandleFunc("POST /api/apps/{slug}/{platform}/testers", s.requireAPI(s.handleApiInviteTesters))
	mux.HandleFunc("GET /api/apps/{slug}/testers", s.requireAPI(s.handleApiGetTesters))
	mux.HandleFunc("GET /api/apps/{slug}/{platform}/testers", s.requireAPI(s.handleApiGetTesters))
	mux.HandleFunc("GET /api/apps/{slug}/tracks", s.requireAPI(s.handleApiListTracks))
	mux.HandleFunc("GET /api/apps/{slug}/{platform}/tracks", s.requireAPI(s.handleApiListTracks))
	mux.HandleFunc("GET /api/apps/{slug}/play/preflight", s.requireAPI(s.handleApiPlayPreflight))
	mux.HandleFunc("GET /api/apps/{slug}/{platform}/play/preflight", s.requireAPI(s.handleApiPlayPreflight))
	mux.HandleFunc("GET /api/play-accounts", s.requireAPI(s.handleApiPlayAccounts))
	mux.HandleFunc("POST /api/play-accounts", s.requireAPI(s.handleApiPlayAccounts))
	mux.HandleFunc("POST /api/play-accounts/delete", s.requireAPI(s.handleApiDeletePlayAccount))
	mux.HandleFunc("POST /api/apps/{slug}/signing", s.requireAPI(s.handleApiSetSigning))
	mux.HandleFunc("POST /api/apps/{slug}/{platform}/signing", s.requireAPI(s.handleApiSetSigning))
	mux.HandleFunc("GET /api/apps/{slug}/signing", s.requireAPI(s.handleApiGetSigning))
	mux.HandleFunc("GET /api/apps/{slug}/{platform}/signing", s.requireAPI(s.handleApiGetSigning))
	mux.HandleFunc("POST /api/apps/{slug}/signing/delete", s.requireAPI(s.handleApiDeleteSigning))
	mux.HandleFunc("POST /api/apps/{slug}/{platform}/signing/delete", s.requireAPI(s.handleApiDeleteSigning))

	// Public (devices): manifest + artifact download need no auth.
	mux.HandleFunc("GET /api/apps/{slug}/manifest", s.handleManifest)
	mux.HandleFunc("GET /api/apps/{slug}/{platform}/manifest", s.handleManifest)
	mux.HandleFunc("GET /apps/{slug}/download", s.handleGetRelease)
	mux.HandleFunc("GET /apps/{slug}/download/{platform}", s.handleGetRelease)
	mux.HandleFunc("GET /apps/{slug}/get", s.handleGetRelease)            // legacy alias
	mux.HandleFunc("GET /apps/{slug}/get/{platform}", s.handleGetRelease) // legacy alias
	mux.HandleFunc("GET /artifacts/{slug}/{platform}/{file}", s.handleArtifact)
	mux.HandleFunc("GET /artifacts/{slug}/{file}", s.handleArtifact) // legacy path shape

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	return mux
}
