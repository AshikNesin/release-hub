package srv

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"srv.exe.dev/db/dbgen"
)

// uiData is embedded in every page template.
type uiData struct {
	Title         string
	Authenticated bool
	Flash         string
	AssetVersion  string // fingerprints embedded CSS/JS for cache-busting URLs
}

// ---- auth pages ----

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.render(w, 200, "login.html", struct {
			uiData
			Error string
		}{uiData{Title: "Sign in", AssetVersion: assetVersion}, ""})
		return
	}
	password := r.FormValue("password")
	if !s.validPassword(password) {
		s.render(w, 401, "login.html", struct {
			uiData
			Error string
		}{uiData{Title: "Sign in", AssetVersion: assetVersion}, "Wrong password"})
		return
	}
	if err := s.setSession(w, r); err != nil {
		http.Error(w, "session error", 500)
		return
	}
	http.Redirect(w, r, "/apps", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// handleFirstRun: on a fresh DB, / redirects here to set the admin password
// (only works while no password exists).
func (s *Server) handleFirstRun(w http.ResponseWriter, r *http.Request) {
	set, err := s.hasPassword()
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	if set {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodGet {
		s.render(w, 200, "firstrun.html", struct {
			uiData
			Error string
		}{uiData{Title: "Setup", AssetVersion: assetVersion}, ""})
		return
	}
	pw, pw2 := r.FormValue("password"), r.FormValue("password2")
	if len(pw) < 8 {
		s.render(w, 400, "firstrun.html", struct {
			uiData
			Error string
		}{uiData{Title: "Setup", AssetVersion: assetVersion}, "Password must be at least 8 characters"})
		return
	}
	if pw != pw2 {
		s.render(w, 400, "firstrun.html", struct {
			uiData
			Error string
		}{uiData{Title: "Setup", AssetVersion: assetVersion}, "Passwords don't match"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err == nil {
		err = dbgen.New(s.DB).SetConfig(r.Context(), dbgen.SetConfigParams{
			Key: "admin_password_hash", Value: string(hash),
		})
	}
	if err != nil {
		s.render(w, 500, "firstrun.html", struct {
			uiData
			Error string
		}{uiData{Title: "Setup", AssetVersion: assetVersion}, "Failed to save: " + err.Error()})
		return
	}
	_ = s.setSession(w, r)
	http.Redirect(w, r, "/apps", http.StatusSeeOther)
}

func (s *Server) hasPassword() (bool, error) {
	ctx := context.Background()
	_, err := dbgen.New(s.DB).GetConfig(ctx, "admin_password_hash")
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// ---- dashboard ----

// handleAppsPage GET /apps — the app list (one row per platform variant;
// platform-less shells listed too, with a nudge to add one).
func (s *Server) handleAppsPage(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	apps, err := q.ListApps(r.Context())
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	type appRow struct {
		Slug, PackageName, Platform, Latest string
		HasPlatforms                        bool
	}
	rows := make([]appRow, 0)
	for _, a := range apps {
		plats, err := q.ListPlatformsByApp(r.Context(), a.ID)
		if err != nil {
			http.Error(w, "db error", 500)
			return
		}
		if len(plats) == 0 {
			// Registered shell with no platform variant yet — still list it
			// (it owns the slug), with a nudge to open its page.
			rows = append(rows, appRow{Slug: a.Slug})
			continue
		}
		for _, p := range plats {
			latest := "—"
			if rels, err := q.LatestReleaseForChannel(r.Context(), dbgen.LatestReleaseForChannelParams{AppPlatformID: p.ID, Channel: "direct"}); err == nil && len(rels) > 0 {
				latest = fmt.Sprintf("%s (%d)", rels[0].VersionName, rels[0].VersionCode)
			}
			rows = append(rows, appRow{a.Slug, p.PackageName, p.Platform, latest, true})
		}
	}
	s.render(w, 200, "apps.html", struct {
		uiData
		Apps []appRow
	}{uiData{Title: "Apps", Authenticated: true, AssetVersion: assetVersion}, rows})
}

// handleCreateAppUI registers the product shell: slug only. Platforms are
// added from the app's own page ("Add platform" form / API), keeping the
// register action a single field.
func (s *Server) handleCreateAppUI(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.FormValue("slug")))
	if !slugRe.MatchString(slug) {
		http.Error(w, "invalid slug", 400)
		return
	}
	if _, err := dbgen.New(s.DB).CreateApp(r.Context(), slug); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "an app with this slug already exists — open its page: /apps/"+slug, 409)
			return
		}
		http.Error(w, "create failed: "+err.Error(), 409)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "rh_flash", Value: "App registered. Add its android/ios platform next.",
		Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30,
	})
	http.Redirect(w, r, "/apps/"+slug, http.StatusSeeOther)
}

// handleAddPlatformUI POST /apps/{slug}/platforms — add another platform
// variant to an existing app from the app detail page.
func (s *Server) handleAddPlatformUI(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, ok := s.appFromSlug(w, r, slug)
	if !ok {
		return
	}
	platform := r.FormValue("platform")
	if platform != "android" && platform != "ios" {
		http.Error(w, "invalid platform", 400)
		return
	}
	pkg := strings.TrimSpace(r.FormValue("packageName"))
	if pkg == "" {
		http.Error(w, "packageName required", 400)
		return
	}
	platID, err := s.addPlatform(r.Context(), app.ID, platform, pkg)
	if err != nil {
		http.Error(w, "add platform failed: "+err.Error(), 409)
		return
	}
	if platform == "android" {
		if _, err := s.generateAndStoreSigning(r.Context(), platID, slug); err != nil {
			slog.Error("auto-signing failed (UI platform add)", "slug", slug, "err", err)
		}
	}
	http.Redirect(w, r, "/apps/"+slug, http.StatusSeeOther)
}

// handleCreateTokenUI creates a token and shows it exactly once via flash.
func (s *Server) handleCreateTokenUI(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name required", 400)
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		http.Error(w, "token error", 500)
		return
	}
	token := "rh_" + hex.EncodeToString(raw)
	if _, err := dbgen.New(s.DB).CreateApiToken(r.Context(), dbgen.CreateApiTokenParams{
		Name: name, TokenHash: hashToken(token),
	}); err != nil {
		http.Error(w, "create failed: "+err.Error(), 500)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "rh_flash", Value: token,
		Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30,
	})
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

// ---- app detail ----

func (s *Server) handleAppDetail(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	q := dbgen.New(s.DB)
	plats, err := q.ListPlatformsByApp(r.Context(), app.ID)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	type relRow struct {
		VersionName, Channel, Notes, URL string
		VersionCode                      int64
		SizeMB                           float64
		CreatedAt                        time.Time
	}
	type platSection struct {
		Platform, PackageName  string
		Releases               []relRow
		ManifestURL, UploadURL string
		HasSigningKey          bool
		SignSha256             string // keystore fingerprint (not secret)
		SignAlias              string // key alias (not secret)
	}
	sections := make([]platSection, 0, len(plats))
	for _, p := range plats {
		releases, err := q.ListReleases(r.Context(), p.ID)
		if err != nil {
			http.Error(w, "db error", 500)
			return
		}
		rows := make([]relRow, 0, len(releases))
		for _, rel := range releases {
			rows = append(rows, relRow{
				VersionName: rel.VersionName, Channel: rel.Channel, Notes: rel.Notes,
				URL:         "/artifacts/" + app.Slug + "/" + p.Platform + "/" + rel.FileName,
				VersionCode: rel.VersionCode,
				SizeMB:      float64(rel.SizeBytes) / (1 << 20),
				CreatedAt:   rel.CreatedAt,
			})
		}
		// Non-confidential signing info: keystore fingerprint + key alias.
		// Passwords stay encrypted and are only released via the API to
		// authenticated CI.
		alias := ""
		if p.SignConfig != "" {
			if cfgJSON, derr := decryptCreds(p.SignConfig); derr == nil {
				var cfg struct {
					KeyAlias string `json:"keyAlias"`
				}
				if json.Unmarshal(cfgJSON, &cfg) == nil {
					alias = cfg.KeyAlias
				}
			}
		}
		sections = append(sections, platSection{
			Platform: p.Platform, PackageName: p.PackageName, Releases: rows,
			ManifestURL:   s.baseURL + "/api/apps/" + app.Slug + "/" + p.Platform + "/manifest",
			UploadURL:     s.baseURL + "/api/apps/" + app.Slug + "/" + p.Platform + "/releases",
			HasSigningKey: p.SignSha256 != "",
			SignSha256:    p.SignSha256,
			SignAlias:     alias,
		})
	}
	s.render(w, 200, "app.html", struct {
		uiData
		App          dbgen.App
		Platforms    []platSection
		SuggestedPkg string // prefill for Add-platform: bundle_prefix + slug
		HasAndroid   bool   // hide android from the picker when it exists
	}{
		uiData{Title: app.Slug, Authenticated: true, AssetVersion: assetVersion},
		app, sections,
		suggestPackage(s.bundlePrefix(r.Context()), app.Slug),
		func() bool {
			for _, p := range plats {
				if p.Platform == "android" {
					return true
				}
			}
			return false
		}(),
	})
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	tmpl, err := parsePageTemplate(s.TemplatesDir, name)
	if err != nil {
		slog.Error("parse template", "name", name, "error", err)
		http.Error(w, "template error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		slog.Error("render template", "name", name, "error", err)
	}
}

// ---- settings ----

// signingSettingsField maps form field -> config key for the cert-subject
// settings shown on the Settings page. Order defines form order.
var signingSettingsFields = []struct{ Field, Key, Label, Hint, Placeholder string }{
	{"org", "sign_org", "Organization (O)", "Company name baked into every generated signing certificate.", "release-hub"},
	{"ou", "sign_ou", "Organizational unit (OU)", "Team or division.", "Mobile"},
	{"locality", "sign_locality", "Locality (L)", "City.", "Bengaluru"},
	{"state", "sign_state", "State (ST)", "State or province.", "Karnataka"},
	{"country", "sign_country", "Country (C)", "Two-letter code.", "IN"},
}

// bundlePrefix reads the configured default package prefix, e.g. "io.nesin".
// Used to prefill the package-name field when adding a platform: prefix+slug
// (io.nesin + tinyfirewall → io.nesin.tinyfirewall).
func (s *Server) bundlePrefix(ctx context.Context) string {
	v, err := dbgen.New(s.DB).GetConfig(ctx, "bundle_prefix")
	if err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(v), ".")
}

// suggestPackage builds the prefilled package name for a new platform.
// Slug is lowercase alnum+dashes; package segments use underscores for dashes.
func suggestPackage(prefix, slug string) string {
	if prefix == "" {
		return ""
	}
	return prefix + "." + strings.ReplaceAll(slug, "-", "_")
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	if r.Method == http.MethodPost {
		// Two independent forms post here (bundle prefix / cert subject).
		// Only overwrite keys the submitted form actually carried; a missing
		// field means "not part of this save", not "clear the value".
		posted := r.PostForm
		if posted == nil {
			if err := r.ParseForm(); err == nil {
				posted = r.PostForm
			}
		}
		for _, f := range signingSettingsFields {
			if !posted.Has(f.Field) {
				continue
			}
			if err := q.SetConfig(r.Context(), dbgen.SetConfigParams{Key: f.Key, Value: strings.TrimSpace(r.FormValue(f.Field))}); err != nil {
				http.Error(w, "save failed: "+err.Error(), 500)
				return
			}
		}
		if posted.Has("bundle_prefix") {
			if err := q.SetConfig(r.Context(), dbgen.SetConfigParams{Key: "bundle_prefix", Value: strings.Trim(strings.TrimSpace(r.FormValue("bundle_prefix")), ".")}); err != nil {
				http.Error(w, "save failed: "+err.Error(), 500)
				return
			}
		}
		http.SetCookie(w, &http.Cookie{
			Name: "rh_flash", Value: "Settings saved. New keys will use this certificate name.",
			Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30,
		})
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	type field struct{ Name, Value, Label, Hint, Placeholder string }
	fields := make([]field, 0, len(signingSettingsFields))
	for _, f := range signingSettingsFields {
		v, _ := q.GetConfig(r.Context(), f.Key)
		fields = append(fields, field{f.Field, v, f.Label, f.Hint, f.Placeholder})
	}
	tokens, _ := q.ListApiTokens(r.Context())
	s.render(w, 200, "settings.html", struct {
		uiData
		Fields       []field
		BundlePrefix string
		Tokens       []dbgen.ApiToken
		NewToken     string
	}{uiData{Title: "Settings", Authenticated: true, AssetVersion: assetVersion},
		fields, s.bundlePrefix(r.Context()), tokens, flashFrom(r)})
}
