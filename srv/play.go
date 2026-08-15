package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/option"
)

// PlayPublisher uploads AABs to Google Play tracks via the official
// Publishing API (androidpublisher v3) using a service-account JSON key.
type PlayPublisher struct {
	svc     *androidpublisher.Service
	pkgName string
}

// NewPlayPublisher reads a service-account key file and builds a client.
// The service account must be linked in Play Console (Users & permissions →
// API access) with release permissions for the app.
func NewPlayPublisher(ctx context.Context, pkgName, credentialsFile string) (*PlayPublisher, error) {
	if pkgName == "" {
		return nil, fmt.Errorf("play: packageName required")
	}
	if _, err := os.Stat(credentialsFile); err != nil {
		return nil, fmt.Errorf("play: credentials file: %w", err)
	}
	b, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("play: read credentials: %w", err)
	}
	var cfg struct {
		ClientEmail string `json:"client_email"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil || cfg.ClientEmail == "" {
		return nil, fmt.Errorf("play: credentials file is not a service-account JSON")
	}
	svc, err := androidpublisher.NewService(ctx,
		option.WithCredentialsFile(credentialsFile),
		option.WithScopes(androidpublisher.AndroidpublisherScope),
	)
	if err != nil {
		return nil, fmt.Errorf("play: create client: %w", err)
	}
	return &PlayPublisher{svc: svc, pkgName: pkgName}, nil
}

// NewPlayPublisherFromJSON builds a client from service-account JSON bytes
// (e.g. stored encrypted in the DB).
func NewPlayPublisherFromJSON(ctx context.Context, pkgName string, creds []byte) (*PlayPublisher, error) {
	tmp, err := os.CreateTemp("", "playcreds-*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(creds); err != nil {
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	return NewPlayPublisher(ctx, pkgName, tmp.Name())
}

// Preflight opens and immediately deletes a Play edit for the package —
// the cheapest authenticated round-trip that doesn't require an existing
// release. Distinguishes credential problems, permission/link problems and
// "app doesn't exist in Play yet".
func (p *PlayPublisher) Preflight(ctx context.Context) error {
	appEdit, err := p.svc.Edits.Insert(p.pkgName, &androidpublisher.AppEdit{}).Context(ctx).Do()
	if err != nil {
		return classifyPlayError(err)
	}
	return p.svc.Edits.Delete(p.pkgName, appEdit.Id).Context(ctx).Do()
}

// classifyPlayError maps raw API errors to actionable hints.
func classifyPlayError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "invalid_grant"), strings.Contains(msg, "invalid_client"):
		return fmt.Errorf("credentials rejected (invalid_grant) — the service-account JSON key is wrong or revoked; upload a fresh key in Settings")
	case strings.Contains(msg, "403"), strings.Contains(msg, "Forbidden"):
		return fmt.Errorf("403 Forbidden — the service account has no access to this package. Either it wasn't invited in Play Console (Users & permissions → Invite new users, grant 'Create and manage releases'), or API access isn't linked yet (Setup → API access), or the app doesn't exist in Play yet")
	case strings.Contains(msg, "404"):
		return fmt.Errorf("404 Not Found — no app with this package exists in Play yet; create it in Play Console first (the API cannot create apps)")
	default:
		return err
	}
}

// trackFor maps hub channels to Play tracks.
func trackFor(channel string) (string, bool) {
	switch channel {
	case "public":
		return "production", true
	case "internal":
		return "internal", true
	case "direct":
		return "", false // direct distribution; no Play involvement
	}
	return "", false
}

// Publish uploads an AAB and assigns it to the Play track for the channel.
// Returns the Play release name.
func (p *PlayPublisher) Publish(ctx context.Context, aabPath, channel, versionName, notes string) (string, error) {
	track, ok := trackFor(channel)
	if !ok {
		return "", fmt.Errorf("play: channel %q has no Play track", channel)
	}
	if !strings.HasSuffix(strings.ToLower(aabPath), ".aab") {
		return "", fmt.Errorf("play: artifact must be an .aab (got %s)", aabPath)
	}

	appEdit, err := p.svc.Edits.Insert(p.pkgName, &androidpublisher.AppEdit{}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("play: open edit: %w", err)
	}
	defer func() {
		if err := p.svc.Edits.Delete(p.pkgName, appEdit.Id).Context(ctx).Do(); err != nil {
			slog.Debug("play: cleanup edit", "error", err)
		}
	}()

	f, err := os.Open(aabPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	bundle, err := p.svc.Edits.Bundles.
		Upload(p.pkgName, appEdit.Id).
		Media(f).
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("play: upload bundle: %w", err)
	}

	releaseName := fmt.Sprintf("%s (%d)", versionName, bundle.VersionCode)
	rel := &androidpublisher.TrackRelease{
		Name:         releaseName,
		Status:       "completed",
		VersionCodes: []int64{bundle.VersionCode},
		ReleaseNotes: []*androidpublisher.LocalizedText{
			{Language: "en-US", Text: notes},
		},
	}
	if notes == "" {
		rel.ReleaseNotes = nil
	}
	_, err = p.svc.Edits.Tracks.
		Update(p.pkgName, appEdit.Id, track, &androidpublisher.Track{Releases: []*androidpublisher.TrackRelease{rel}}).
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("play: set track %s: %w", track, err)
	}
	if _, err := p.svc.Edits.Commit(p.pkgName, appEdit.Id).Context(ctx).Do(); err != nil {
		return "", fmt.Errorf("play: commit: %w", err)
	}
	return releaseName, nil
}
