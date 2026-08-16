package srv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/googleapi"
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
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		if strings.Contains(err.Error(), "invalid_grant") || strings.Contains(err.Error(), "invalid_client") {
			return fmt.Errorf("credentials rejected (invalid_grant) — the service-account JSON key is wrong or revoked; upload a fresh key in Settings")
		}
		return err
	}
	switch gerr.Code {
	case 401:
		return fmt.Errorf("%s — the service-account JSON key is wrong or revoked; upload a fresh key in Settings", gerr.Message)
	case 403:
		// The API-disabled case has its own precise remedy (the enable URL);
		// don't bury it under the generic no-access hints.
		if strings.Contains(gerr.Message, "has not been used in project") ||
			strings.Contains(gerr.Message, "it is disabled") {
			return fmt.Errorf("%s — open the URL in this message and click Enable, wait a few minutes, then test again. (This is the Google Play Android Developer API switch in the service account's Cloud project; normally the Play Console → Setup → API access link enables it automatically.)", gerr.Message)
		}
		return fmt.Errorf("%s — no access to this package. Causes in order of likelihood: (1) app not created in Play yet (the API cannot create apps), (2) service account not invited in Play Console → Users & permissions → Invite new users with 'Create and manage releases', (3) API access not linked in Play Console → Setup → API access. Note: a newly invited service account or fresh API link can take up to 24h (usually minutes) to take effect.", gerr.Message)
	case 404:
		return fmt.Errorf("%s — no app with this package exists in Play yet; create it in Play Console first (the API cannot create apps)", gerr.Message)
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
// For the internal track, testers (Google Group addresses) are set in the
// same edit so a release and its tester list always land together.
// Returns the Play release name.
func (p *PlayPublisher) Publish(ctx context.Context, aabPath, channel, versionName, notes string, testers []string) (string, error) {
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
	// Explicit media type: without it the type is sniffed from the temp
	// file (application/zip for .aab), which Play rejects with
	// "Media type 'application/zip' is not supported".
	bundle, err := p.svc.Edits.Bundles.
		Upload(p.pkgName, appEdit.Id).
		Media(f, googleapi.ContentType("application/octet-stream")).
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
	// Internal track: set the tester list (Google Groups) in the same edit.
	// Without this the track's tester list stays empty and the opt-in link
	// (play.google.com/apps/testing/<pkg>) shows "app not available" for
	// everyone — the API cannot invite individual emails, only groups.
	if track == "internal" {
		if _, err := p.svc.Edits.Testers.
			Update(p.pkgName, appEdit.Id, track, &androidpublisher.Testers{GoogleGroups: testers}).
			Context(ctx).
			Do(); err != nil {
			return "", fmt.Errorf("play: set testers: %w", err)
		}
	}
	if _, err := p.svc.Edits.Commit(p.pkgName, appEdit.Id).Context(ctx).Do(); err != nil {
		return "", fmt.Errorf("play: commit: %w", err)
	}
	return releaseName, nil
}

// SetTesters replaces the tester list (Google Group addresses) on a Play
// track without touching releases — the manual "invite testers" action.
// The API cannot invite individual emails, only Google Groups; groups can
// be created free at groups.google.com.
func (p *PlayPublisher) SetTesters(ctx context.Context, track string, groups []string) error {
	appEdit, err := p.svc.Edits.Insert(p.pkgName, &androidpublisher.AppEdit{}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("play: open edit: %w", err)
	}
	defer func() {
		if err := p.svc.Edits.Delete(p.pkgName, appEdit.Id).Context(ctx).Do(); err != nil {
			slog.Debug("play: cleanup edit", "error", err)
		}
	}()
	if _, err := p.svc.Edits.Testers.
		Update(p.pkgName, appEdit.Id, track, &androidpublisher.Testers{GoogleGroups: groups}).
		Context(ctx).
		Do(); err != nil {
		return fmt.Errorf("play: set testers on %s: %w", track, err)
	}
	if _, err := p.svc.Edits.Commit(p.pkgName, appEdit.Id).Context(ctx).Do(); err != nil {
		return fmt.Errorf("play: commit: %w", err)
	}
	return nil
}
