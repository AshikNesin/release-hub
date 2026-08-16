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
	"strconv"
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

// shareRow is one copyable distribution link on the app page ("share"
// row): direct = latest APK on this hub; internal/public = Play opt-in /
// listing links derived from the package name.
type shareRow struct {
	Label, URL string
}

// playInternalTestingURL and playStoreURL are the user-facing links for a
// package on Google Play. internaltesting/ is the opt-in page testers use to
// join the internal-testing track; details is the public listing.
func playInternalTestingURL(pkg string) string {
	return "https://play.google.com/apps/testing/" + pkg
}

func playStoreURL(pkg string) string {
	return "https://play.google.com/store/apps/details?id=" + pkg
}

// handleAppDetail GET /apps/{slug} — per-platform sections: signing info,
// manifest URL, share links, Play config, and the release table.
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
		Platform, PackageName, LatestVersion string
		Releases                             []relRow
		ManifestURL, UploadURL               string
		Shares                               []shareRow
		BetaTesters                          string // hub-wide tester groups (display; the invite replaces the track list)
		PruneKeepOverride                    string // platform retention override ("" = inherit)
		PruneKeepEffective                   string // what actually applies, for display
		HasSigningKey                        bool
		SignSha256                           string // keystore fingerprint (not secret)
		SignAlias                            string // key alias (not secret)
		PlayEnabled                          bool
		PlayAccountID                        int64  // linked shared service account (0 = none)
		PlayEmail                            string // service-account email (identifier, not secret)
	}
	sections := make([]platSection, 0, len(plats))
	betaTesters := strings.Join(s.playTesters(r.Context()), ", ")
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
		// Latest across channels (highest versionCode) for the platform header.
		latest := ""
		if len(rows) > 0 {
			latest = rows[0].VersionName
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
		acctID, acctEmail := s.playAccountInfo(p)
		override := ""
		if p.PruneKeep != nil {
			override = strconv.FormatInt(*p.PruneKeep, 10)
		}
		// Share links: one stable /download URL per channel — direct always
		// (latest APK on this hub); the two Play channels only while Play
		// publishing is enabled for the platform.
		dl := s.baseURL + "/apps/" + app.Slug + "/download?channel="
		shares := []shareRow{{Label: "direct", URL: dl + "direct"}}
		if p.PlayEnabled == 1 {
			shares = append(shares,
				shareRow{Label: "internal", URL: dl + "internal"},
				shareRow{Label: "public", URL: dl + "public"})
		}
		sections = append(sections, platSection{
			Platform: p.Platform, PackageName: p.PackageName, LatestVersion: latest, Releases: rows,
			ManifestURL:       s.baseURL + "/api/apps/" + app.Slug + "/" + p.Platform + "/manifest",
			UploadURL:         s.baseURL + "/api/apps/" + app.Slug + "/" + p.Platform + "/releases",
			Shares:            shares,
			BetaTesters:       betaTesters,
			PruneKeepOverride: override,
			PruneKeepEffective: inheritLabel(func() *int64 {
				if p.PruneKeep != nil {
					return p.PruneKeep
				}
				k := int64(s.pruneConfig(r.Context(), p))
				return &k
			}()),
			HasSigningKey: p.SignSha256 != "",
			SignSha256:    p.SignSha256,
			SignAlias:     alias,
			PlayEnabled:   p.PlayEnabled == 1,
			PlayAccountID: acctID,
			PlayEmail:     acctEmail,
		})
	}
	type accountOption struct {
		ID    int64
		Email string
	}
	accounts := make([]accountOption, 0)
	for _, acct := range s.mustListPlayAccounts(r) {
		accounts = append(accounts, accountOption{acct.ID, playEmail(acct.Credentials)})
	}
	// Tab routing: ?tab=releases|distribution|configuration (default
	// releases). Unknown values fall back to releases, not an error.
	tab := r.URL.Query().Get("tab")
	switch tab {
	case "distribution", "configuration":
	default:
		tab = "releases"
	}
	s.render(w, 200, "app.html", struct {
		uiData
		App          dbgen.App
		Tab          string
		Platforms    []platSection
		SuggestedPkg string // prefill for Add-platform: bundle_prefix + slug
		HasAndroid   bool   // hide android from the picker when it exists
		PlayAccounts []accountOption
	}{
		uiData{Title: app.Slug, Authenticated: true, Flash: flashFrom(r), AssetVersion: assetVersion},
		app, tab, sections,
		suggestPackage(s.bundlePrefix(r.Context()), app.Slug),
		func() bool {
			for _, p := range plats {
				if p.Platform == "android" {
					return true
				}
			}
			return false
		}(),
		accounts,
	})
}

// playEmail extracts the service-account identifier from stored (encrypted)
// Play credentials, for display. Anything else stays sealed in the DB.
func playEmail(encCreds string) string {
	if encCreds == "" {
		return ""
	}
	raw, err := decryptCreds(encCreds)
	if err != nil {
		return ""
	}
	var probe struct {
		ClientEmail string `json:"client_email"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		return ""
	}
	return probe.ClientEmail
}

// playAccountInfo resolves a platform's linked shared account to
// (accountID, email). Zero values when not linked.
func (s *Server) playAccountInfo(p dbgen.AppPlatform) (int64, string) {
	if p.PlayAccountID == nil {
		return 0, ""
	}
	acct, err := dbgen.New(s.DB).PlayAccountByID(context.Background(), *p.PlayAccountID)
	if err != nil {
		return *p.PlayAccountID, ""
	}
	return acct.ID, playEmail(acct.Credentials)
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

// backTo reads the ?back= query param (validated against known tabs) and
// builds the redirect target after a platform-scoped POST, so actions return
// to the tab they were triggered from.
func backTo(slug string, r *http.Request) string {
	switch r.FormValue("back") {
	case "distribution", "configuration":
		return "/apps/" + slug + "?tab=" + r.FormValue("back")
	}
	return "/apps/" + slug
}

// handlePlayConfigUI POST /apps/{slug}/platforms/{platform}/play —
// session-auth twin of the bearer API: enable this platform for Play against
// a shared service account (picked with account=<id>, or a credentials file
// which also creates/replaces the shared account), or disable with
// enable=false. Redirects back with a flash.
func (s *Server) handlePlayConfigUI(w http.ResponseWriter, r *http.Request) {
	plat, ok := s.platformFromRequest(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	back := backTo(r.PathValue("slug"), r)
	setFlash := func(msg string) {
		http.SetCookie(w, &http.Cookie{
			Name: "rh_flash", Value: msg, Path: "/", HttpOnly: true,
			SameSite: http.SameSiteLaxMode, MaxAge: 30,
		})
	}
	if r.FormValue("enable") == "false" {
		if err := dbgen.New(s.DB).SetPlayAccountForPlatform(r.Context(), dbgen.SetPlayAccountForPlatformParams{
			PlayEnabled: 0, PlayAccountID: nil, ID: plat.ID,
		}); err != nil {
			http.Error(w, "disable failed: "+err.Error(), 500)
			return
		}
		setFlash("Google Play publishing disabled.")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	var acctID int64
	var email string
	if f, hdr, err := r.FormFile("file"); err == nil && hdr.Size > 0 {
		f.Close()
		acctID, email, err = s.upsertPlayAccountFromRequest(r)
		if err != nil {
			setFlash("Play setup failed: " + err.Error())
			http.Redirect(w, r, back, http.StatusSeeOther)
			return
		}
	} else if idStr := strings.TrimSpace(r.FormValue("account")); idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			setFlash("Play setup failed: bad account id.")
			http.Redirect(w, r, back, http.StatusSeeOther)
			return
		}
		acct, err := dbgen.New(s.DB).PlayAccountByID(r.Context(), id)
		if err != nil {
			setFlash("Play setup failed: unknown service account.")
			http.Redirect(w, r, back, http.StatusSeeOther)
			return
		}
		acctID, email = acct.ID, playEmail(acct.Credentials)
	} else {
		setFlash("Play setup failed: pick a service account or upload a JSON key.")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if err := dbgen.New(s.DB).SetPlayAccountForPlatform(r.Context(), dbgen.SetPlayAccountForPlatformParams{
		PlayEnabled: 1, PlayAccountID: &acctID, ID: plat.ID,
	}); err != nil {
		http.Error(w, "enable failed: "+err.Error(), 500)
		return
	}
	setFlash("Google Play publishing enabled via " + email + ".")
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// handlePlayPreflightUI GET /apps/{slug}/platforms/{platform}/play/check —
// session-auth wrapper around the preflight API; returns JSON for the
// button's fetch() on the app page.
func (s *Server) handlePlayPreflightUI(w http.ResponseWriter, r *http.Request) {
	s.handleApiPlayPreflight(w, r)
}

// handleInviteTestersUI POST /apps/{slug}/platforms/{platform}/testers
// Session-auth wrapper that runs the API invite and redirects back with a
// flash (the button is a form, not fetch — testers sync is infrequent).
func (s *Server) handleInviteTestersUI(w http.ResponseWriter, r *http.Request) {
	back := backTo(r.PathValue("slug"), r)
	setFlash := func(msg string) {
		http.SetCookie(w, &http.Cookie{
			Name: "rh_flash", Value: msg, Path: "/", HttpOnly: true,
			SameSite: http.SameSiteLaxMode, MaxAge: 30,
		})
	}
	plat, ok := s.platformFromRequest(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	groups := s.playTesters(r.Context())
	if len(groups) == 0 {
		setFlash("No beta testers configured — add Google Group addresses in Settings.")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if plat.PlayEnabled == 0 || plat.PlayAccountID == nil {
		setFlash("Enable Play publishing first — the invite uses its service account.")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	creds, err := s.playAccountCreds(*plat.PlayAccountID)
	if err != nil {
		setFlash("Play account failed: " + err.Error())
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	pub, err := NewPlayPublisherFromJSON(r.Context(), plat.PackageName, creds)
	if err != nil {
		setFlash("Play setup failed: " + err.Error())
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if err := pub.SetTesters(r.Context(), "internal", groups); err != nil {
		setFlash("Inviting testers failed: " + err.Error())
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	slog.Info("play testers updated (UI)", "app", r.PathValue("slug"), "groups", len(groups))
	setFlash(fmt.Sprintf("Invited %d tester group(s) to the internal track.", len(groups)))
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// handleDeleteReleaseUI POST /apps/{slug}/platforms/{platform}/releases/{versionCode}/delete
// Delete a direct-channel release from the Releases tab (row button).
func (s *Server) handleDeleteReleaseUI(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	back := backTo(slug, r)
	setFlash := func(msg string) {
		http.SetCookie(w, &http.Cookie{
			Name: "rh_flash", Value: msg, Path: "/", HttpOnly: true,
			SameSite: http.SameSiteLaxMode, MaxAge: 30,
		})
	}
	plat, ok := s.platformFromRequest(w, r, slug)
	if !ok {
		return
	}
	versionCode, err := strconv.ParseInt(r.PathValue("versionCode"), 10, 64)
	if err != nil {
		setFlash("Bad version code.")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	rel, err := dbgen.New(s.DB).ReleaseByAppAndCode(r.Context(), dbgen.ReleaseByAppAndCodeParams{
		AppPlatformID: plat.ID, VersionCode: versionCode,
	})
	if err != nil {
		setFlash("No such release.")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if rel.Channel != "direct" {
		setFlash("Only direct-channel releases can be deleted (internal/public are managed by Play).")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	found, err := s.deleteRelease(r.Context(), plat, slug, versionCode)
	if err != nil || !found {
		setFlash("Delete failed: " + err.Error())
	} else {
		setFlash(fmt.Sprintf("Deleted release %s (%d).", rel.VersionName, versionCode))
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// handleRetentionUI POST /apps/{slug}/platforms/{platform}/retention
// Set the platform's prune override (blank = inherit the global setting).
func (s *Server) handleRetentionUI(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	back := backTo(slug, r)
	setFlash := func(msg string) {
		http.SetCookie(w, &http.Cookie{
			Name: "rh_flash", Value: msg, Path: "/", HttpOnly: true,
			SameSite: http.SameSiteLaxMode, MaxAge: 30,
		})
	}
	plat, ok := s.platformFromRequest(w, r, slug)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		setFlash("Bad form.")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	keep, err := parseKeep(r.FormValue("keep"))
	if err != nil {
		setFlash("Retention: " + err.Error())
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if err := dbgen.New(s.DB).SetPruneKeep(r.Context(), dbgen.SetPruneKeepParams{
		PruneKeep: keep, ID: plat.ID,
	}); err != nil {
		setFlash("Retention: save failed — " + err.Error())
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	msg := fmt.Sprintf("Retention set: keep %s.", inheritLabel(keep))
	setFlash(msg)
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// inheritLabel renders a prune-keep value for flash messages.
func inheritLabel(keep *int64) string {
	if keep == nil {
		return "the global setting (inherit)"
	}
	if *keep == 0 {
		return "everything (never prune)"
	}
	return fmt.Sprintf("newest %d direct releases", *keep)
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
		// Shared Play service accounts (separate form from the settings forms).
		if r.URL.Path == "/settings/play" {
			s.handlePlayAccountsUI(w, r)
			return
		}
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
		// Beta testers: Google Group addresses pushed to the Play internal
		// track on every internal publish (and via the app page's invite
		// button). Normalized to comma-separated on save.
		if posted.Has("play_testers") {
			groups := normalizeTesterGroups(r.FormValue("play_testers"))
			if err := q.SetConfig(r.Context(), dbgen.SetConfigParams{Key: "play_testers", Value: strings.Join(groups, ", ")}); err != nil {
				http.Error(w, "save failed: "+err.Error(), 500)
				return
			}
		}
		// Global release retention: keep newest N direct releases per
		// platform (0 = keep everything; blank also = keep everything).
		// Platforms can override this on their Configuration tab.
		if posted.Has("prune_keep") {
			v := strings.TrimSpace(r.FormValue("prune_keep"))
			if v != "" {
				if n, err := strconv.Atoi(v); err != nil || n < 0 {
					http.Error(w, "release retention must be a number ≥ 0", 400)
					return
				}
			}
			if err := q.SetConfig(r.Context(), dbgen.SetConfigParams{Key: "prune_keep", Value: v}); err != nil {
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
	betaTesters, _ := q.GetConfig(r.Context(), "play_testers")
	pruneKeep, _ := q.GetConfig(r.Context(), "prune_keep")
	type playAcctRow struct {
		ID, Email string
		Platforms []string // "slug (platform)" using this account
	}
	playRows := make([]playAcctRow, 0)
	for _, acct := range s.mustListPlayAccounts(r) {
		row := playAcctRow{ID: strconv.FormatInt(acct.ID, 10), Email: playEmail(acct.Credentials)}
		plats, err := dbgen.New(s.DB).ListPlatformsWithPlay(r.Context())
		if err == nil {
			for _, p := range plats {
				if p.PlayAccountID != nil && *p.PlayAccountID == acct.ID {
					row.Platforms = append(row.Platforms, strconv.FormatInt(int64(p.AppID), 10)+"/"+p.Platform)
				}
			}
		}
		playRows = append(playRows, row)
	}
	s.render(w, 200, "settings.html", struct {
		uiData
		Fields       []field
		BundlePrefix string
		BetaTesters  string
		PruneKeep    string
		Tokens       []dbgen.ApiToken
		NewToken     string
		PlayAccounts []playAcctRow
	}{uiData{Title: "Settings", Authenticated: true, AssetVersion: assetVersion},
		fields, s.bundlePrefix(r.Context()), betaTesters, pruneKeep, tokens, flashFrom(r), playRows})
}

// normalizeTesterGroups splits a comma/newline/space-separated tester list
// into clean group addresses (deduped, order preserved).
func normalizeTesterGroups(v string) []string {
	seen := make(map[string]bool)
	groups := make([]string, 0, 4)
	for _, g := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == ';' }) {
		if g = strings.ToLower(strings.TrimSpace(g)); g != "" && !seen[g] {
			seen[g] = true
			groups = append(groups, g)
		}
	}
	return groups
}

// handlePlayAccountsUI POST /settings/play — add/replace/delete the shared
// Play service accounts from the Settings page.
func (s *Server) handlePlayAccountsUI(w http.ResponseWriter, r *http.Request) {
	setFlash := func(msg string) {
		http.SetCookie(w, &http.Cookie{
			Name: "rh_flash", Value: msg, Path: "/", HttpOnly: true,
			SameSite: http.SameSiteLaxMode, MaxAge: 30,
		})
	}
	if r.FormValue("action") == "delete" {
		id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
		if err != nil {
			setFlash("Play account: bad id.")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}
		// Clear flags BEFORE deleting — the FK's ON DELETE SET NULL would
		// otherwise null play_account_id first and orphan play_enabled=1 rows.
		if _, err := s.DB.Exec(`UPDATE app_platforms SET play_enabled = 0 WHERE play_account_id = ?`, id); err != nil {
			setFlash("Play account: clearing linked apps failed — " + err.Error())
		} else if err := dbgen.New(s.DB).DeletePlayAccount(r.Context(), id); err != nil {
			setFlash("Play account: delete failed — " + err.Error())
		} else {
			setFlash("Play service account removed.")
		}
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	_, email, err := s.upsertPlayAccountFromRequest(r)
	if err != nil {
		setFlash("Play account failed: " + err.Error())
	} else {
		setFlash("Service account " + email + " saved — enable it per app from the app's page.")
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}
