package srv

// OIDC keyless publishing — HTTP surface.
//
//   - POST /api/oidc/exchange   (public): CI's OIDC identity token → a
//     short-lived hub bearer token scoped to one app's publish endpoints.
//   - /api/v1/oidc/grants …     (session): grant CRUD for the UI.
//
// Grants live in oidc_grants (migration 011): issuer + subject → one app.
// Minted tokens are api_tokens rows with scope_slug set; requireAPI refuses
// them anywhere outside the granted app's publish endpoints and deletes
// expired ones on sight.

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
)

// handleApiOidcExchange POST /api/oidc/exchange
//
// form/JSON: token — the CI job's OIDC identity token (audience must be
// "release-hub"). On success: {token, expiresIn} where token is a normal
// rh_ bearer usable for the publish endpoints of the granted app only.
func (s *Server) handleApiOidcExchange(w http.ResponseWriter, r *http.Request) {
	raw := r.Header.Get("X-OIDC-Token")
	if raw == "" {
		raw = r.FormValue("token")
	}
	grant, err := s.verifyOIDCToken(r.Context(), raw)
	if err != nil {
		slog.Warn("oidc exchange rejected", "err", err.Error())
		w.Header().Set("WWW-Authenticate", `Bearer realm="release-hub", error="invalid_token"`)
		writeErr(w, http.StatusUnauthorized, "oidc token rejected: "+err.Error())
		return
	}

	token, err := randomToken("rh")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	q := dbgen.New(s.DB)
	expires := time.Now().Add(exchangeTTL)
	if _, err := q.CreateScopedApiToken(r.Context(), dbgen.CreateScopedApiTokenParams{
		Name:         "oidc: " + grant.AppSlug,
		TokenHash:    hashToken(token),
		ScopeSlug:    grant.AppSlug,
		ExpiresAt:    &expires,
		OidcGrantID:  &grant.ID,
	}); err != nil {
		writeErr(w, 500, "mint failed: "+err.Error())
		return
	}
	now := time.Now()
	_ = q.TouchOidcGrant(r.Context(), dbgen.TouchOidcGrantParams{LastUsedAt: &now, ID: grant.ID})
	writeJSON(w, 200, map[string]any{
		"token":     token,
		"expiresIn": int(exchangeTTL.Seconds()),
		"app":       grant.AppSlug,
	})
}

// ---- v1: grant management (UI) ----

// handleV1ListOidcGrants GET /api/v1/apps/{slug}/oidc/grants
func (s *Server) handleV1ListOidcGrants(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	grants, err := dbgen.New(s.DB).ListOidcGrantsByApp(r.Context(), app.ID)
	if err != nil {
		writeErr(w, 500, "db error")
		return
	}
	out := make([]map[string]any, 0, len(grants))
	for _, g := range grants {
		row := map[string]any{
			"id":        g.ID,
			"issuer":    g.Issuer,
			"subject":   g.Subject,
			"name":      g.Name,
			"createdAt": g.CreatedAt.UTC().Format(time.RFC3339),
		}
		if g.LastUsedAt != nil {
			v := g.LastUsedAt.UTC().Format(time.RFC3339)
			row["lastUsedAt"] = v
		}
		out = append(out, row)
	}
	writeJSON(w, 200, out)
}

// handleV1CreateOidcGrant POST /api/v1/apps/{slug}/oidc/grants
// {issuer, subject, name?} — one grant per identity; one repo → one app.
func (s *Server) handleV1CreateOidcGrant(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	issuer := strings.TrimSpace(r.FormValue("issuer"))
	subject := strings.TrimSpace(r.FormValue("subject"))
	name := strings.TrimSpace(r.FormValue("name"))
	if issuer == "" || subject == "" {
		writeErr(w, 400, "issuer and subject are required")
		return
	}
	if u, err := url.ParseRequestURI(issuer); err != nil ||
		(u.Scheme != "https" && !isLocalhost(u.Host)) {
		writeErr(w, 400, "issuer must be an https URL, e.g. https://token.actions.githubusercontent.com")
		return
	}
	if _, err := dbgen.New(s.DB).CreateOidcGrant(r.Context(), dbgen.CreateOidcGrantParams{
		AppID: app.ID, Issuer: issuer, Subject: subject, Name: name,
	}); err != nil {
		writeErr(w, 409, "a grant for this issuer+subject already exists (one repo maps to one app)")
		return
	}
	s.handleV1ListOidcGrants(w, r)
}

// isLocalhost permits http issuers for local development (tests, make run).
func isLocalhost(host string) bool {
	h := host
	if i := strings.LastIndex(h, ":"); i != -1 {
		h = h[:i]
	}
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

// handleV1DeleteOidcGrant DELETE /api/v1/oidc/grants/{id} — also kills the
// grant's outstanding minted tokens.
func (s *Server) handleV1DeleteOidcGrant(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid grant id")
		return
	}
	q := dbgen.New(s.DB)
	if err := q.DeleteApiTokensByGrant(r.Context(), &id); err != nil {
		writeErr(w, 500, "db error")
		return
	}
	if err := q.DeleteOidcGrant(r.Context(), id); err != nil {
		writeErr(w, 500, "db error")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
