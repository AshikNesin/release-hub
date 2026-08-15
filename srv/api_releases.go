package srv

// API handlers: release upload/listing and Play publishing.

import (
	"context"
	"encoding/json"
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

// ---- API: releases ----

// handleApiUpload POST /api/apps/{slug}/releases
//
// multipart form:
//
//	file        artifact (.apk/.aab/.ipa)
//	channel     direct | internal | public   (default direct)
//	versionName human version (default derived from versionCode)
//	versionCode integer; required for android APKs on direct (the
//	           on-device BuildConfig.VERSION_CODE, i.e. ABI-adjusted);
//	           optional otherwise (auto = max+1)
//	notes       release notes
func (s *Server) handleApiUpload(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	plat, ok := s.platformFromRequest(w, r, slug)
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
	maxCode, err := maxVersionCode(r.Context(), q, plat.ID)
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

	// Stream to storage while hashing; key = <slug>/<platform>/<versionCode>_<filename>
	fileName := fmt.Sprintf("%d_%s", versionCode, filepath.Base(hdr.Filename))
	key := slug + "/" + plat.Platform + "/" + fileName
	size, sha, err := s.storage.Save(r.Context(), key, file)
	if err != nil {
		writeErr(w, 500, "store artifact: "+err.Error())
		return
	}

	if _, err := q.CreateRelease(r.Context(), dbgen.CreateReleaseParams{
		AppPlatformID: plat.ID, VersionCode: versionCode, VersionName: versionName,
		Channel: channel, Notes: r.FormValue("notes"),
		Sha256: sha, SizeBytes: size, FileName: fileName,
	}); err != nil {
		_ = s.storage.Delete(r.Context(), key)
		writeErr(w, 500, "record release: "+err.Error())
		return
	}
	dlURL, err := s.storage.PublicURL(r.Context(), key)
	if err != nil {
		writeErr(w, 500, "sign url: "+err.Error())
		return
	}
	resp := map[string]any{
		"slug": slug, "platform": plat.Platform, "versionCode": versionCode, "versionName": versionName,
		"channel": channel, "sha256": sha, "size": size,
		"apkUrl": dlURL,
	}

	// Optional Play Store publishing for .aab artifacts when this app has a
	// service-account key in PlayCredsDir.
	if playRelease, err := s.publishToPlay(r, plat, key, channel, versionName, r.FormValue("notes")); err != nil {
		slog.Warn("play publish failed", "app", slug, "error", err)
		resp["playError"] = err.Error() // stored + served; release itself succeeded
	} else if playRelease != "" {
		resp["playRelease"] = playRelease
	}
	writeJSON(w, 201, resp)
}

// publishToPlay pushes an .aab to Google Play when the app has Play enabled
// and credentials stored. Returns ("", nil) when not applicable.
func (s *Server) publishToPlay(r *http.Request, plat dbgen.AppPlatform, key, channel, versionName, notes string) (string, error) {
	if plat.PlayEnabled == 0 || plat.PlayCredentials == "" ||
		!strings.HasSuffix(strings.ToLower(key), ".aab") {
		return "", nil
	}
	creds, err := decryptCreds(plat.PlayCredentials)
	if err != nil {
		return "", fmt.Errorf("decrypt play credentials: %w", err)
	}
	pub, err := NewPlayPublisherFromJSON(r.Context(), plat.PackageName, creds)
	if err != nil {
		return "", err
	}
	// Materialize the artifact to a temp file for the uploader.
	tmp, err := os.CreateTemp("", "hub-aab-*.aab")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	rc, err := s.storage.Open(r.Context(), key)
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
	slog.Info("play publish ok", "app", plat.AppID, "channel", channel, "release", release)
	return release, nil
}

// handleApiSetPlay POST /api/apps/{slug}/play
// multipart form: file=<service-account.json> to enable; empty file field
// (or enable=false) disables Play publishing for the platform.
func (s *Server) handleApiSetPlay(w http.ResponseWriter, r *http.Request) {
	plat, ok := s.platformFromRequest(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	if r.FormValue("enable") == "false" {
		if err := dbgen.New(s.DB).SetPlayConfig(r.Context(), dbgen.SetPlayConfigParams{
			PlayEnabled: 0, PlayCredentials: "", ID: plat.ID,
		}); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"playEnabled": false})
		return
	}
	email, err := s.storePlayCredentials(r, plat.ID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"playEnabled": true, "serviceAccount": email})
}

// storePlayCredentials validates a multipart file field as service-account
// JSON, encrypts it and enables Play publishing for the platform row.
// Returns the service-account email for confirmation messages.
func (s *Server) storePlayCredentials(r *http.Request, platformID int64) (string, error) {
	f, hdr, err := r.FormFile("file")
	if err != nil || hdr.Size == 0 {
		return "", fmt.Errorf("multipart file field with service-account JSON required (or enable=false to disable)")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	// validate it parses as a service-account JSON before storing
	var probe struct {
		ClientEmail string `json:"client_email"`
	}
	if json.Unmarshal(raw, &probe) != nil || probe.ClientEmail == "" {
		return "", fmt.Errorf("file is not a service-account JSON (missing client_email)")
	}
	enc, err := encryptCreds(raw)
	if err != nil {
		return "", err
	}
	if err := dbgen.New(s.DB).SetPlayConfig(r.Context(), dbgen.SetPlayConfigParams{
		PlayEnabled: 1, PlayCredentials: enc, ID: platformID,
	}); err != nil {
		return "", err
	}
	return probe.ClientEmail, nil
}

// handleApiReleases GET /api/apps/{slug}/releases
func (s *Server) handleApiReleases(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	plat, ok := s.platformFromRequest(w, r, slug)
	if !ok {
		return
	}
	releases, err := dbgen.New(s.DB).ListReleases(r.Context(), plat.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(releases))
	for _, rel := range releases {
		u, _ := s.storage.PublicURL(r.Context(), slug+"/"+plat.Platform+"/"+rel.FileName)
		out = append(out, map[string]any{
			"versionCode": rel.VersionCode, "versionName": rel.VersionName,
			"channel": rel.Channel, "sha256": rel.Sha256, "size": rel.SizeBytes,
			"url":       u,
			"createdAt": rel.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, 200, out)
}
