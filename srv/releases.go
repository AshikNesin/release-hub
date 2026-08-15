package srv

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// maxVersionCode wraps the COALESCE query that sqlc types as interface{}.
func maxVersionCode(ctx context.Context, q *dbgen.Queries, appID int64) (int64, error) {
	v, err := q.MaxVersionCode(ctx, appID)
	if err != nil {
		return 0, err
	}
	switch n := v.(type) {
	case int64:
		return n, nil
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("unexpected max version type %T", v)
	}
}

const maxUploadBytes = 512 << 20 // 512 MiB per artifact

// releaseManifest is the wire format consumed by Tiny Firewall's AppUpdater
// (BuildConfig.UPDATE_URL). Keep it stable: versionCode must be the ABI-
// adjusted on-device value for android APKs served on the direct channel.
type releaseManifest struct {
	VersionCode int    `json:"versionCode"`
	VersionName string `json:"versionName"`
	ApkURL      string `json:"apkUrl"`
	Sha256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

// ---- API: apps ----

// handleApiListApps GET /api/apps
func (s *Server) handleApiListApps(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	apps, err := q.ListApps(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	type appOut struct {
		Slug        string `json:"slug"`
		PackageName string `json:"packageName"`
		Platform    string `json:"platform"`
	}
	out := make([]appOut, 0, len(apps))
	for _, a := range apps {
		out = append(out, appOut{a.Slug, a.PackageName, a.Platform})
	}
	writeJSON(w, 200, out)
}

// handleApiCreateApp POST /api/apps  (form: slug, packageName, platform)
func (s *Server) handleApiCreateApp(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.FormValue("slug")))
	pkg := strings.TrimSpace(r.FormValue("packageName"))
	platform := strings.TrimSpace(r.FormValue("platform"))
	if platform == "" {
		platform = "android"
	}
	if !slugRe.MatchString(slug) {
		writeErr(w, 400, "invalid slug: lowercase letters, digits and dashes")
		return
	}
	if pkg == "" {
		writeErr(w, 400, "packageName required")
		return
	}
	if platform != "android" && platform != "ios" {
		writeErr(w, 400, "platform must be android or ios")
		return
	}
	q := dbgen.New(s.DB)
	res, err := q.CreateApp(r.Context(), dbgen.CreateAppParams{Slug: slug, PackageName: pkg, Platform: platform})
	if err != nil {
		writeErr(w, 409, "app already exists or invalid: "+err.Error())
		return
	}
	out := map[string]string{"slug": slug, "channel": platform}
	// Android apps: auto-generate a dedicated signing keystore so the app is
	// deploy-ready immediately — no manual keytool/upload step. Developer
	// experience: register the app, set HUB_TOKEN, run deploy.sh. The key is
	// generated once at creation and never rotates (rotation would break
	// installed-base update signature continuity); uploading a keystore via
	// /signing still overrides it for apps with an existing key history.
	if platform == "android" {
		appID, _ := res.LastInsertId()
		p12, cfg, certPEM, gerr := generateKeystore("Android App: " + slug)
		if gerr != nil {
			slog.Error("auto-signing generation failed", "slug", slug, "err", gerr)
			writeJSON(w, 201, out) // app exists; key can be uploaded later
			return
		}
		cfgJSON, _ := json.Marshal(cfg)
		encKS, e1 := encryptCreds(p12)
		encCfg, e2 := encryptCreds(cfgJSON)
		if e1 != nil || e2 != nil {
			slog.Error("auto-signing encrypt failed", "slug", slug)
			writeJSON(w, 201, out)
			return
		}
		sum := sha256.Sum256(p12)
		if err := q.SetSigningConfig(r.Context(), dbgen.SetSigningConfigParams{
			SignKeystore: encKS, SignConfig: encCfg,
			SignSha256: hex.EncodeToString(sum[:]), ID: appID,
		}); err != nil {
			slog.Error("auto-signing store failed", "slug", slug, "err", err)
			writeJSON(w, 201, out)
			return
		}
		_ = certPEM
		out["signingKey"] = "generated"
		out["signingSha256"] = hex.EncodeToString(sum[:])
	}
	writeJSON(w, 201, out)
}

// appFromSlug resolves ?app= or path param to an app row.
func (s *Server) appFromSlug(w http.ResponseWriter, r *http.Request, slug string) (dbgen.App, bool) {
	app, err := dbgen.New(s.DB).AppBySlug(r.Context(), slug)
	if err != nil {
		writeErr(w, 404, "unknown app: "+slug)
		return dbgen.App{}, false
	}
	return app, true
}

// ---- API: releases ----

// handleApiUpload POST /api/apps/{slug}/releases
//
// multipart form:
//   file        artifact (.apk/.aab/.ipa)
//   channel     direct | internal | public   (default direct)
//   versionName human version (default derived from versionCode)
//   versionCode integer; required for android APKs on direct (the
//              on-device BuildConfig.VERSION_CODE, i.e. ABI-adjusted);
//              optional otherwise (auto = max+1)
//   notes       release notes
func (s *Server) handleApiUpload(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, ok := s.appFromSlug(w, r, slug)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeErr(w, 400, "bad multipart form (max 512MB): "+err.Error())
		return
	}
	channel := r.FormValue("channel")
	if channel == "" {
		channel = "direct"
	}
	switch channel {
	case "public", "internal", "direct":
	default:
		writeErr(w, 400, "channel must be direct, internal or public")
		return
	}
	q := dbgen.New(s.DB)
	maxCode, err := maxVersionCode(r.Context(), q, app.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	versionCode := int64(0)
	if v := r.FormValue("versionCode"); v != "" {
		versionCode, err = strconv.ParseInt(v, 10, 64)
		if err != nil || versionCode <= 0 {
			writeErr(w, 400, "invalid versionCode")
			return
		}
	} else {
		versionCode = maxCode + 1
	}
	versionName := strings.TrimSpace(r.FormValue("versionName"))
	if versionName == "" {
		versionName = fmt.Sprintf("%d", versionCode)
	}
	if versionCode <= maxCode {
		writeErr(w, 409, fmt.Sprintf("versionCode must be > %d (latest for this app)", maxCode))
		return
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "missing file field")
		return
	}
	defer file.Close()

	// Stream to storage while hashing; key = <slug>/<versionCode>_<filename>
	fileName := fmt.Sprintf("%d_%s", versionCode, filepath.Base(hdr.Filename))
	key := app.Slug + "/" + fileName
	size, sha, err := s.Storage.Save(r.Context(), key, file)
	if err != nil {
		writeErr(w, 500, "store artifact: "+err.Error())
		return
	}

	if _, err := q.CreateRelease(r.Context(), dbgen.CreateReleaseParams{
		AppID: app.ID, VersionCode: versionCode, VersionName: versionName,
		Channel: channel, Notes: r.FormValue("notes"),
		Sha256: sha, SizeBytes: size, FileName: fileName,
	}); err != nil {
		_ = s.Storage.Delete(r.Context(), key)
		writeErr(w, 500, "record release: "+err.Error())
		return
	}
	dlURL, err := s.Storage.PublicURL(r.Context(), key)
	if err != nil {
		writeErr(w, 500, "sign url: "+err.Error())
		return
	}
	resp := map[string]any{
		"slug": app.Slug, "versionCode": versionCode, "versionName": versionName,
		"channel": channel, "sha256": sha, "size": size,
		"apkUrl": dlURL,
	}

	// Optional Play Store publishing for .aab artifacts when this app has a
	// service-account key in PlayCredsDir.
	if playRelease, err := s.publishToPlay(r, app, key, channel, versionName, r.FormValue("notes")); err != nil {
		slog.Warn("play publish failed", "app", app.Slug, "error", err)
		resp["playError"] = err.Error() // stored + served; release itself succeeded
	} else if playRelease != "" {
		resp["playRelease"] = playRelease
	}
	writeJSON(w, 201, resp)
}

// publishToPlay pushes an .aab to Google Play when the app has Play enabled
// and credentials stored. Returns ("", nil) when not applicable.
func (s *Server) publishToPlay(r *http.Request, app dbgen.App, key, channel, versionName, notes string) (string, error) {
	if app.PlayEnabled == 0 || app.PlayCredentials == "" ||
		!strings.HasSuffix(strings.ToLower(key), ".aab") {
		return "", nil
	}
	creds, err := decryptCreds(app.PlayCredentials)
	if err != nil {
		return "", fmt.Errorf("decrypt play credentials: %w", err)
	}
	pub, err := NewPlayPublisherFromJSON(r.Context(), app.PackageName, creds)
	if err != nil {
		return "", err
	}
	// Materialize the artifact to a temp file for the uploader.
	tmp, err := os.CreateTemp("", "hub-aab-*.aab")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	rc, err := s.Storage.Open(r.Context(), key)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	if _, err := io.Copy(tmp, rc); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	release, err := pub.Publish(r.Context(), tmp.Name(), channel, versionName, notes)
	if err != nil {
		return "", err
	}
	slog.Info("play publish ok", "app", app.Slug, "channel", channel, "release", release)
	return release, nil
}

// handleApiSetPlay POST /api/apps/{slug}/play
// multipart form: file=<service-account.json> to enable; empty file field
// (or enable=false) disables Play publishing for the app.
func (s *Server) handleApiSetPlay(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	q := dbgen.New(s.DB)

	if r.FormValue("enable") == "false" {
		if err := q.SetPlayConfig(r.Context(), dbgen.SetPlayConfigParams{
			PlayEnabled: 0, PlayCredentials: "", ID: app.ID,
		}); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"playEnabled": false})
		return
	}

	f, hdr, err := r.FormFile("file")
	if err != nil || hdr.Size == 0 {
		writeErr(w, 400, "multipart file field with service-account JSON required (or enable=false to disable)")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		writeErr(w, 400, "read file: "+err.Error())
		return
	}
	// validate it parses as a service-account JSON before storing
	var probe struct {
		ClientEmail string `json:"client_email"`
	}
	if json.Unmarshal(raw, &probe) != nil || probe.ClientEmail == "" {
		writeErr(w, 400, "file is not a service-account JSON (missing client_email)")
		return
	}
	enc, err := encryptCreds(raw)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := q.SetPlayConfig(r.Context(), dbgen.SetPlayConfigParams{
		PlayEnabled: 1, PlayCredentials: enc, ID: app.ID,
	}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"playEnabled": true, "serviceAccount": probe.ClientEmail})
}

// handleApiReleases GET /api/apps/{slug}/releases
func (s *Server) handleApiReleases(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	releases, err := dbgen.New(s.DB).ListReleases(r.Context(), app.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(releases))
	for _, rel := range releases {
		u, _ := s.Storage.PublicURL(r.Context(), app.Slug+"/"+rel.FileName)
		out = append(out, map[string]any{
			"versionCode": rel.VersionCode, "versionName": rel.VersionName,
			"channel": rel.Channel, "sha256": rel.Sha256, "size": rel.SizeBytes,
			"url": u,
			"createdAt": rel.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, 200, out)
}

// handleManifest GET /api/apps/{slug}/manifest?channel=direct
// Wire-compatible with Tiny Firewall's AppUpdater (UPDATE_URL). 404 until a
// release exists on the channel.
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	channel := r.URL.Query().Get("channel")
	if channel == "" {
		channel = "direct"
	}
	releases, err := dbgen.New(s.DB).LatestReleaseForChannel(r.Context(), dbgen.LatestReleaseForChannelParams{
		AppID: app.ID, Channel: channel,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 500, err.Error())
		return
	}
	// LIMIT 1 with no match returns zero rows, not ErrNoRows — guard the
	// slice access or a channel with no releases panics the handler.
	if len(releases) == 0 {
		writeErr(w, 404, "no release on channel "+channel)
		return
	}
	rel := releases[0]
	dlURL, err := s.Storage.PublicURL(r.Context(), app.Slug+"/"+rel.FileName)
	if err != nil {
		writeErr(w, 500, "sign url: "+err.Error())
		return
	}
	writeJSON(w, 200, releaseManifest{
		VersionCode: int(rel.VersionCode),
		VersionName: rel.VersionName,
		ApkURL:      dlURL,
		Sha256:      rel.Sha256,
		Size:        rel.SizeBytes,
	})
}

// handleArtifact GET /artifacts/{slug}/{file} — public. For local storage
// this streams from disk; for S3 it 302s to a presigned URL so the hub never
// proxies bucket traffic.
func (s *Server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	file := filepath.Base(r.PathValue("file"))
	key := slug + "/" + file
	if _, isLocal := s.Storage.(*LocalStorage); !isLocal {
		u, err := s.Storage.PublicURL(r.Context(), key)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, u, http.StatusFound)
		return
	}
	rc, err := s.Storage.Open(r.Context(), key)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rc.Close()
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, rc)
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
