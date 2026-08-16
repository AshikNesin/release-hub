package srv

// Public device-facing endpoints: the update manifest consumed by installed
// apps (BuildConfig.UPDATE_URL) and artifact downloads. No auth — these are
// hit by every device on every update check.

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"path/filepath"

	"srv.exe.dev/db/dbgen"
)

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

// handleManifest GET /api/apps/{slug}/manifest?channel=direct
// Wire-compatible with Tiny Firewall's AppUpdater (UPDATE_URL). 404 until a
// release exists on the channel.
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	plat, ok := s.platformFromRequest(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	channel := r.URL.Query().Get("channel")
	if channel == "" {
		channel = "direct"
	}
	releases, err := dbgen.New(s.DB).LatestReleaseForChannel(r.Context(), dbgen.LatestReleaseForChannelParams{
		AppPlatformID: plat.ID, Channel: channel,
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
	dlURL, err := s.storage.PublicURL(r.Context(), r.PathValue("slug")+"/"+plat.Platform+"/"+rel.FileName)
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

// handleGetRelease GET /apps/{slug}/get and /apps/{slug}/get/{platform}
// Stable, shareable "latest on channel" link: 302 to the newest release's
// artifact URL on ?channel= (default direct). 404 when the channel has no
// release yet. This is the URL the app page's share row copies — it stays
// valid across versions, unlike a versioned artifact URL.
func (s *Server) handleGetRelease(w http.ResponseWriter, r *http.Request) {
	plat, ok := s.platformFromRequest(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	channel := r.URL.Query().Get("channel")
	if channel == "" {
		channel = "direct"
	}
	releases, err := dbgen.New(s.DB).LatestReleaseForChannel(r.Context(), dbgen.LatestReleaseForChannelParams{
		AppPlatformID: plat.ID, Channel: channel,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 500, err.Error())
		return
	}
	if len(releases) == 0 {
		writeErr(w, 404, "no release on channel "+channel)
		return
	}
	rel := releases[0]
	dlURL, err := s.storage.PublicURL(r.Context(), r.PathValue("slug")+"/"+plat.Platform+"/"+rel.FileName)
	if err != nil {
		writeErr(w, 500, "sign url: "+err.Error())
		return
	}
	http.Redirect(w, r, dlURL, http.StatusFound)
}

// handleArtifact GET /artifacts/{slug}/{file} — public. For local storage
// this streams from disk; for S3 it 302s to a presigned URL so the hub never
// proxies bucket traffic.
func (s *Server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	file := filepath.Base(r.PathValue("file"))
	// New layout: /artifacts/{slug}/{platform}/{file}. Legacy two-segment
	// paths (pre-platform restructure) are served as-is via platform="".
	platform := r.PathValue("platform")
	key := slug + "/" + platform + "/" + file
	if platform == "" {
		key = slug + "/" + file
	}
	if _, isLocal := s.storage.(*LocalStorage); !isLocal {
		u, err := s.storage.PublicURL(r.Context(), key)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, u, http.StatusFound)
		return
	}
	rc, err := s.storage.Open(r.Context(), key)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rc.Close()
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, rc)
}
