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
//	channel     direct | internal | alpha | beta | public   (default direct)
//	            internal → internal testing · alpha → closed testing
//	            (auto-created track) · beta → open testing · public →
//	            production
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
	case "public", "beta", "alpha", "internal", "direct":
	default:
		// Legacy spellings (open, closed, closed:<name>) still resolve.
		if _, ok := trackFor(channel); !ok {
			writeErr(w, 400, "channel must be direct, internal, alpha, beta or public")
			return
		}
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
	// Retention: keep only the newest N direct releases (Settings, or the
	// platform's override). Runs after the upload is fully committed.
	if channel == "direct" {
		if n := s.pruneDirectReleases(r.Context(), plat, slug, s.pruneConfig(r.Context(), plat)); n > 0 {
			resp["pruned"] = n
		}
	}
	writeJSON(w, 201, resp)
}

// handleApiDeleteRelease DELETE /api/apps/{slug}/{platform}/releases/{versionCode}
// (and POST …/releases/{versionCode}/delete — the UI form twin).
// Direct-channel releases only: Play channels mirror what Play published.
func (s *Server) handleApiDeleteRelease(w http.ResponseWriter, r *http.Request) {
	plat, ok := s.platformFromRequest(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	versionCode, err := strconv.ParseInt(r.PathValue("versionCode"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid versionCode")
		return
	}
	rel, err := dbgen.New(s.DB).ReleaseByAppAndCode(r.Context(), dbgen.ReleaseByAppAndCodeParams{
		AppPlatformID: plat.ID, VersionCode: versionCode,
	})
	if err != nil {
		writeErr(w, 404, "no such release")
		return
	}
	if rel.Channel != "direct" {
		writeErr(w, 400, "only direct-channel releases can be deleted (Play channels are managed by Play)")
		return
	}
	found, err := s.deleteRelease(r.Context(), plat, r.PathValue("slug"), versionCode)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !found {
		writeErr(w, 404, "no such release")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// publishToPlay pushes an .aab to Google Play when the platform is enabled
// for Play and a shared service account exists. Returns ("", nil) when
// not applicable.
func (s *Server) publishToPlay(r *http.Request, plat dbgen.AppPlatform, key, channel, versionName, notes string) (string, error) {
	if plat.PlayEnabled == 0 || plat.PlayAccountID == nil ||
		!strings.HasSuffix(strings.ToLower(key), ".aab") {
		return "", nil
	}
	creds, err := s.playAccountCreds(*plat.PlayAccountID)
	if err != nil {
		return "", fmt.Errorf("play account: %w", err)
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
	release, err := pub.Publish(r.Context(), tmp.Name(), channel, versionName, notes, s.playTesters(r.Context()))
	if err != nil {
		return "", err
	}
	slog.Info("play publish ok", "app", plat.AppID, "channel", channel, "release", release)
	return release, nil
}

// playTesters reads the hub-wide beta-tester list (Google Group addresses,
// comma/newline separated) from config. Empty when unset.
func (s *Server) playTesters(ctx context.Context) []string {
	v, err := dbgen.New(s.DB).GetConfig(ctx, "play_testers")
	if err != nil {
		return nil
	}
	return normalizeTesterGroups(v)
}

// testerEmails reads the hub-wide individual tester emails (comma/newline
// separated) from config. Empty when unset. These are NOT pushable to Play
// (the API takes Google Groups only) — they're the copy-paste source for
// the Console's tester email lists.
func (s *Server) testerEmails(ctx context.Context) []string {
	v, err := dbgen.New(s.DB).GetConfig(ctx, "play_tester_emails")
	if err != nil {
		return nil
	}
	return normalizeTesterGroups(v)
}

// handleApiInviteTesters POST /api/apps/{slug}/{platform}/testers
// Push the hub-wide beta-tester groups to a Play closed testing track
// (?channel=alpha, the hub's closed testing track). Requires Play publishing
// enabled (needs the service account).
func (s *Server) handleApiInviteTesters(w http.ResponseWriter, r *http.Request) {
	plat, ok := s.platformFromRequest(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	if plat.PlayEnabled == 0 || plat.PlayAccountID == nil {
		writeErr(w, 400, "Play publishing is not enabled for this platform")
		return
	}
	// Tester groups only land on closed testing tracks — Google rejects
	// them on internal ("Cannot set tester group on an internal track")
	// and production/open manage testers in Play Console instead.
	channel := r.FormValue("channel")
	if channel == "" {
		channel = "alpha"
	}
	if !trackIsClosed(channel) {
		writeErr(w, 400, "testers can only be set on the alpha (closed testing) track")
		return
	}
	groups := s.playTesters(r.Context())
	if len(groups) == 0 {
		writeErr(w, 400, "no beta testers configured — add Google Group addresses in Settings")
		return
	}
	creds, err := s.playAccountCreds(*plat.PlayAccountID)
	if err != nil {
		writeErr(w, 500, "play account: "+err.Error())
		return
	}
	pub, err := NewPlayPublisherFromJSON(r.Context(), plat.PackageName, creds)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	track, err := pub.ensureClosedTrack(r.Context(), channel)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	if err := pub.SetTesters(r.Context(), track, groups); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	slog.Info("play testers updated", "app", r.PathValue("slug"), "track", track, "groups", len(groups))
	writeJSON(w, 200, map[string]any{"ok": true, "track": track, "groups": groups})
}

// handleApiGetTesters GET /api/apps/{slug}/{platform}/testers
// The hub's tester inventory: groups (pushable to Play closed tracks) and
// individual emails (Console-only; the Play API cannot accept personal
// addresses). Lets a helper script or CI paste the email list into Play
// Console's tester email lists.
func (s *Server) handleApiGetTesters(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.platformFromRequest(w, r, r.PathValue("slug")); !ok {
		return
	}
	writeJSON(w, 200, map[string]any{
		"groups": s.playTesters(r.Context()),
		"emails": s.testerEmails(r.Context()),
		"hint":   "groups: pushable to the alpha (closed testing) track (POST channel=alpha); emails: paste into Play Console email lists — the API cannot accept individual addresses",
	})
}

// handleApiListTracks GET /api/apps/{slug}/{platform}/tracks
// Read-only Play track inventory (name + current releases) — answers
// "what's on each of my Play tracks?"
// without opening Play Console.
func (s *Server) handleApiListTracks(w http.ResponseWriter, r *http.Request) {
	plat, ok := s.platformFromRequest(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	if plat.PlayEnabled == 0 || plat.PlayAccountID == nil {
		writeErr(w, 400, "Play publishing is not enabled for this platform")
		return
	}
	creds, err := s.playAccountCreds(*plat.PlayAccountID)
	if err != nil {
		writeErr(w, 500, "play account: "+err.Error())
		return
	}
	pub, err := NewPlayPublisherFromJSON(r.Context(), plat.PackageName, creds)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	tracks, err := pub.ListTracks(r.Context())
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	type trackInfo struct {
		Track    string   `json:"track"`
		IsClosed bool     `json:"isClosed"`
		Channel  string   `json:"channel"`  // the hub channel value to use
		Releases []string `json:"releases"` // current releases on the track
	}
	out := make([]trackInfo, 0, len(tracks))
	for _, t := range tracks {
		info := trackInfo{Track: t.Track, Channel: "closed:" + t.Track}
		switch t.Track {
		case "production":
			info.Channel, info.IsClosed = "public", false
		case "beta":
			info.Channel, info.IsClosed = "beta", false
		case "internal", "qa":
			info.Channel, info.IsClosed = "internal", false
		case alphaTrack:
			// The hub's closed testing track — the "alpha" channel.
			info.Channel, info.IsClosed = "alpha", true
		default:
			info.IsClosed = true // free-form name = closed testing track
		}
		for _, rel := range t.Releases {
			info.Releases = append(info.Releases, rel.Name)
		}
		out = append(out, info)
	}
	writeJSON(w, 200, out)
}

// handleApiSetPlay POST /api/apps/{slug}/play
// Toggle Play publishing for a platform against a shared service account.
// Forms: account=<id> [&enable=true] to enable, enable=false to disable.
// Legacy form still accepted: file=<service-account.json> creates a shared
// account and enables the platform on it.
func (s *Server) handleApiSetPlay(w http.ResponseWriter, r *http.Request) {
	plat, ok := s.platformFromRequest(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	if r.FormValue("enable") == "false" {
		if err := dbgen.New(s.DB).SetPlayAccountForPlatform(r.Context(), dbgen.SetPlayAccountForPlatformParams{
			PlayEnabled: 0, PlayAccountID: nil, ID: plat.ID,
		}); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"playEnabled": false})
		return
	}
	// Legacy: credentials file in the request → create/replace shared account.
	if f, hdr, err := r.FormFile("file"); err == nil && hdr.Size > 0 {
		f.Close()
		acctID, email, err := s.upsertPlayAccountFromRequest(r)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if err := dbgen.New(s.DB).SetPlayAccountForPlatform(r.Context(), dbgen.SetPlayAccountForPlatformParams{
			PlayEnabled: 1, PlayAccountID: &acctID, ID: plat.ID,
		}); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"playEnabled": true, "serviceAccount": email})
		return
	}
	// New form: enable against an existing shared account.
	account := strings.TrimSpace(r.FormValue("account"))
	if account == "" {
		writeErr(w, 400, "account=<id> required (or file=<service-account.json> to add one); enable=false disables)")
		return
	}
	acctID, err := strconv.ParseInt(account, 10, 64)
	if err != nil {
		writeErr(w, 400, "account must be a numeric id")
		return
	}
	acct, err := dbgen.New(s.DB).PlayAccountByID(r.Context(), acctID)
	if err != nil {
		writeErr(w, 404, "unknown play account "+account)
		return
	}
	if err := dbgen.New(s.DB).SetPlayAccountForPlatform(r.Context(), dbgen.SetPlayAccountForPlatformParams{
		PlayEnabled: 1, PlayAccountID: &acct.ID, ID: plat.ID,
	}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"playEnabled": true, "serviceAccount": playEmail(acct.Credentials)})
}

// playAccountCreds decrypts a shared play account's credentials.
func (s *Server) playAccountCreds(id int64) ([]byte, error) {
	acct, err := dbgen.New(s.DB).PlayAccountByID(context.Background(), id)
	if err != nil {
		return nil, fmt.Errorf("account %d not found: %w", id, err)
	}
	creds, err := decryptCreds(acct.Credentials)
	if err != nil {
		return nil, fmt.Errorf("decrypt play credentials: %w", err)
	}
	return creds, nil
}

// handleApiPlayPreflight GET/POST /api/apps/{slug}/play/preflight —
// verify the platform's Play wiring (account → credentials → API access →
// app exists) without uploading anything. Returns {ok, detail}.
func (s *Server) handleApiPlayPreflight(w http.ResponseWriter, r *http.Request) {
	plat, ok := s.platformFromRequest(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	if plat.PlayEnabled == 0 || plat.PlayAccountID == nil {
		writeJSON(w, 200, map[string]any{"ok": false,
			"detail": "Play publishing is not enabled for this app (no linked service account)."})
		return
	}
	creds, err := s.playAccountCreds(*plat.PlayAccountID)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "detail": err.Error()})
		return
	}
	pub, err := NewPlayPublisherFromJSON(r.Context(), plat.PackageName, creds)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "detail": err.Error()})
		return
	}
	if err := pub.Preflight(r.Context()); err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "detail": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true,
		"detail": fmt.Sprintf("Play wiring OK for %s — ready to publish .aab releases.", plat.PackageName)})
}

// upsertPlayAccountFromRequest reads the multipart file field, validates it
// as service-account JSON and stores it as a shared play account. If the
// same service-account email already exists it is replaced. Returns the
// account id and email.
func (s *Server) upsertPlayAccountFromRequest(r *http.Request) (int64, string, error) {
	f, hdr, err := r.FormFile("file")
	if err != nil || hdr.Size == 0 {
		return 0, "", fmt.Errorf("multipart file field with service-account JSON required")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return 0, "", fmt.Errorf("read file: %w", err)
	}
	var probe struct {
		ClientEmail string `json:"client_email"`
	}
	if json.Unmarshal(raw, &probe) != nil || probe.ClientEmail == "" {
		return 0, "", fmt.Errorf("file is not a service-account JSON (missing client_email)")
	}
	// Replace an existing account with the same email (key rotation).
	for _, acct := range s.mustListPlayAccounts(r) {
		if playEmail(acct.Credentials) == probe.ClientEmail {
			if err := dbgen.New(s.DB).DeletePlayAccount(r.Context(), acct.ID); err != nil {
				return 0, "", err
			}
			break
		}
	}
	enc, err := encryptCreds(raw)
	if err != nil {
		return 0, "", err
	}
	acct, err := dbgen.New(s.DB).CreatePlayAccount(r.Context(), dbgen.CreatePlayAccountParams{
		Label: probe.ClientEmail, Credentials: enc,
	})
	if err != nil {
		return 0, "", err
	}
	return acct.ID, probe.ClientEmail, nil
}

func (s *Server) mustListPlayAccounts(r *http.Request) []dbgen.PlayAccount {
	accts, err := dbgen.New(s.DB).ListPlayAccounts(r.Context())
	if err != nil {
		slog.Warn("list play accounts", "error", err)
		return nil
	}
	return accts
}

// handleApiPlayAccounts manages the hub-wide service accounts.
// POST /api/play-accounts   (file=<service-account.json>, label?) → create
// GET  /api/play-accounts   → list (ids + emails only, no credentials)
// POST /api/play-accounts/delete  (id=<id>) → remove
func (s *Server) handleApiPlayAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		type row struct {
			ID    int64  `json:"id"`
			Email string `json:"email"`
			Label string `json:"label"`
		}
		rows := make([]row, 0)
		for _, acct := range s.mustListPlayAccounts(r) {
			rows = append(rows, row{acct.ID, playEmail(acct.Credentials), acct.Label})
		}
		writeJSON(w, 200, rows)
	case http.MethodPost:
		id, email, err := s.upsertPlayAccountFromRequest(r)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"id": id, "serviceAccount": email})
	default:
		writeErr(w, 405, "method not allowed")
	}
}

// handleApiDeletePlayAccount POST /api/play-accounts/delete (id)
func (s *Server) handleApiDeletePlayAccount(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
	if err != nil {
		writeErr(w, 400, "id required")
		return
	}
	if _, err := dbgen.New(s.DB).PlayAccountByID(r.Context(), id); err != nil {
		writeErr(w, 404, "unknown play account")
		return
	}
	if _, err := s.DB.Exec(`UPDATE app_platforms SET play_enabled = 0 WHERE play_account_id = ?`, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// Delete after clearing flags: the FK's ON DELETE SET NULL would
	// otherwise null play_account_id first and orphan play_enabled=1 rows.
	if err := dbgen.New(s.DB).DeletePlayAccount(r.Context(), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"deleted": true})
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
