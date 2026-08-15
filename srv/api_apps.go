package srv

// API handlers: app + platform registration and API tokens. Slug/platform
// resolution helpers live here as well.

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"srv.exe.dev/db/dbgen"
)

// ---- API: apps ----

// handleApiListApps GET /api/apps
func (s *Server) handleApiListApps(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	apps, err := q.ListApps(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	type platOut struct {
		Platform    string `json:"platform"`
		PackageName string `json:"packageName"`
	}
	type appOut struct {
		Slug      string    `json:"slug"`
		Platforms []platOut `json:"platforms"`
	}
	out := make([]appOut, 0, len(apps))
	for _, a := range apps {
		plats, err := q.ListPlatformsByApp(r.Context(), a.ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		po := make([]platOut, 0, len(plats))
		for _, p := range plats {
			po = append(po, platOut{p.Platform, p.PackageName})
		}
		out = append(out, appOut{a.Slug, po})
	}
	writeJSON(w, 200, out)
}

// handleApiCreateApp POST /api/apps  (form: slug)
//
// Registers the product shell — slug only. Platforms (which carry the
// package name, signing key, releases) are added with
// POST /api/apps/{slug}/platforms, so one slug covers both the android and
// ios versions of the same product.
func (s *Server) handleApiCreateApp(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.FormValue("slug")))
	if !slugRe.MatchString(slug) {
		writeErr(w, 400, "invalid slug: lowercase letters, digits and dashes")
		return
	}
	if _, err := dbgen.New(s.DB).CreateApp(r.Context(), slug); err != nil {
		writeErr(w, 409, "app already exists or invalid: "+err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"slug": slug})
}

// handleApiAddPlatform POST /api/apps/{slug}/platforms
// (form: platform, packageName) — adds e.g. the ios variant of an app that
// already exists as android. Android platforms get a generated signing key.
func (s *Server) handleApiAddPlatform(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, ok := s.appFromSlug(w, r, slug)
	if !ok {
		return
	}
	platform := strings.TrimSpace(r.FormValue("platform"))
	pkg := strings.TrimSpace(r.FormValue("packageName"))
	if platform != "android" && platform != "ios" {
		writeErr(w, 400, "platform must be android or ios")
		return
	}
	if pkg == "" {
		writeErr(w, 400, "packageName required")
		return
	}
	platID, err := s.addPlatform(r.Context(), app.ID, platform, pkg)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	out := map[string]string{"slug": slug, "platform": platform}
	if platform == "android" {
		sum, gerr := s.generateAndStoreSigning(r.Context(), platID, slug)
		if gerr != nil {
			slog.Error("auto-signing failed", "slug", slug, "platform", platform, "err", gerr)
			writeJSON(w, 201, out)
			return
		}
		out["signingKey"] = "generated"
		out["signingSha256"] = sum
	}
	writeJSON(w, 201, out)
}

// addPlatform creates a platform variant row for an app (UNIQUE(app_id,
// platform)). Returns the new platform id.
func (s *Server) addPlatform(ctx context.Context, appID int64, platform, pkg string) (int64, error) {
	res, err := dbgen.New(s.DB).CreateAppPlatform(ctx, dbgen.CreateAppPlatformParams{
		AppID: appID, Platform: platform, PackageName: pkg,
	})
	if err != nil {
		return 0, fmt.Errorf("platform already exists or invalid: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// platformFromRequest resolves (slug, platform) to the platform row used by
// signing/play/upload/manifest endpoints. Platform comes from the path
// ({platform}) or ?platform=, defaulting to android.
func (s *Server) platformFromRequest(w http.ResponseWriter, r *http.Request, slug string) (dbgen.AppPlatform, bool) {
	platform := r.PathValue("platform")
	if platform == "" {
		platform = r.URL.Query().Get("platform")
	}
	if platform == "" {
		platform = "android"
	}
	if platform != "android" && platform != "ios" {
		writeErr(w, 400, "platform must be android or ios")
		return dbgen.AppPlatform{}, false
	}
	pl, err := dbgen.New(s.DB).PlatformBySlugAndPlatform(r.Context(), dbgen.PlatformBySlugAndPlatformParams{
		Slug: slug, Platform: platform,
	})
	if err != nil {
		writeErr(w, 404, "unknown app/platform: "+slug+"/"+platform)
		return dbgen.AppPlatform{}, false
	}
	return pl, true
}

// ---- API: tokens (UI-only actions also exposed here) ----

// appFromSlug resolves ?app= or path param to an app row.
func (s *Server) appFromSlug(w http.ResponseWriter, r *http.Request, slug string) (dbgen.App, bool) {
	app, err := dbgen.New(s.DB).AppBySlug(r.Context(), slug)
	if err != nil {
		writeErr(w, 404, "unknown app: "+slug)
		return dbgen.App{}, false
	}
	return app, true
}

// ---- API: tokens (UI-only actions also exposed here) ----

// handleApiCreateToken POST /api/tokens (form: name) — bearer required.
// Returns the raw token exactly once.
func (s *Server) handleApiCreateToken(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		writeErr(w, 400, "name required")
		return
	}
	token, err := randomToken("rh")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if _, err := dbgen.New(s.DB).CreateApiToken(r.Context(), dbgen.CreateApiTokenParams{
		Name: name, TokenHash: hashToken(token),
	}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"name": name, "token": token})
}
