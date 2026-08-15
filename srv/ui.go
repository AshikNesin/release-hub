package srv

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"context"
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
}

// ---- auth pages ----

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.render(w, 200, "login.html", struct {
			uiData
			Error string
		}{uiData{Title: "Sign in"}, ""})
		return
	}
	password := r.FormValue("password")
	if !s.validPassword(password) {
		s.render(w, 401, "login.html", struct {
			uiData
			Error string
		}{uiData{Title: "Sign in"}, "Wrong password"})
		return
	}
	if err := s.setSession(w, r); err != nil {
		http.Error(w, "session error", 500)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
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
		}{uiData{Title: "Setup"}, ""})
		return
	}
	pw, pw2 := r.FormValue("password"), r.FormValue("password2")
	if len(pw) < 8 {
		s.render(w, 400, "firstrun.html", struct {
			uiData
			Error string
		}{uiData{Title: "Setup"}, "Password must be at least 8 characters"})
		return
	}
	if pw != pw2 {
		s.render(w, 400, "firstrun.html", struct {
			uiData
			Error string
		}{uiData{Title: "Setup"}, "Passwords don't match"})
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
		}{uiData{Title: "Setup"}, "Failed to save: " + err.Error()})
		return
	}
	_ = s.setSession(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
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

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	apps, err := q.ListApps(r.Context())
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	type appRow struct {
		Slug, PackageName, Platform, Latest string
	}
	rows := make([]appRow, 0, len(apps))
	for _, a := range apps {
		latest := "—"
		if rels, err := q.LatestReleaseForChannel(r.Context(), dbgen.LatestReleaseForChannelParams{AppID: a.ID, Channel: "api-share"}); err == nil && len(rels) > 0 {
			latest = fmt.Sprintf("%s (%d)", rels[0].VersionName, rels[0].VersionCode)
		}
		rows = append(rows, appRow{a.Slug, a.PackageName, a.Platform, latest})
	}
	tokens, _ := q.ListApiTokens(r.Context())
	s.render(w, 200, "apps.html", struct {
		uiData
		Apps   []appRow
		Tokens []dbgen.ApiToken
	}{uiData{Title: "Apps", Authenticated: true}, rows, tokens})
}

func (s *Server) handleCreateAppUI(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.FormValue("slug")))
	if !slugRe.MatchString(slug) {
		http.Error(w, "invalid slug", 400)
		return
	}
	_, err := dbgen.New(s.DB).CreateApp(r.Context(), dbgen.CreateAppParams{
		Slug: slug, PackageName: r.FormValue("packageName"), Platform: r.FormValue("platform"),
	})
	if err != nil {
		http.Error(w, "create failed: "+err.Error(), 409)
		return
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
		Name: "rh_flash", Value: "New token (copy now, shown once): " + token,
		Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ---- app detail ----

func (s *Server) handleAppDetail(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	q := dbgen.New(s.DB)
	releases, err := q.ListReleases(r.Context(), app.ID)
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
	rows := make([]relRow, 0, len(releases))
	for _, rel := range releases {
		rows = append(rows, relRow{
			VersionName: rel.VersionName, Channel: rel.Channel, Notes: rel.Notes,
			URL: "/artifacts/" + app.Slug + "/" + rel.FileName,
			VersionCode: rel.VersionCode,
			SizeMB:      float64(rel.SizeBytes) / (1 << 20),
			CreatedAt:   rel.CreatedAt,
		})
	}
	s.render(w, 200, "app.html", struct {
		uiData
		App         dbgen.App
		Releases    []relRow
		ManifestURL string
		UploadURL   string
	}{
		uiData{Title: app.Slug, Authenticated: true},
		app, rows,
		s.BaseURL + "/api/apps/" + app.Slug + "/manifest",
		s.BaseURL + "/api/apps/" + app.Slug + "/releases",
	})
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := renderTemplate(w, s.TemplatesDir, name, data); err != nil {
		slog.Warn("render template", "name", name, "error", err)
	}
}
