package srv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
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

// trackFor maps hub channels to Play track identifiers.
//
// Fixed tracks: production, open testing ("beta" — note: Google's reserved
// id for the OPEN track) and internal testing ("internal"; alias "qa").
// Closed testing tracks are created on demand (Play's edits.tracks.create,
// type closedTesting) or reused when they already exist — the hub's simple
// model is ONE closed track named "beta-testers", addressed as channel
// "closed". The explicit form "closed:<name>" remains for power users who
// created additional closed tracks in Play Console.
func trackFor(channel string) (string, bool) {
	switch channel {
	case "public":
		return "production", true
	case "open":
		return "beta", true
	case "internal":
		return "internal", true
	case "direct":
		return "", false // direct distribution; no Play involvement
	}
	// "closed" — the hub's default closed testing track.
	if channel == "closed" {
		return closedDefaultTrack, true
	}
	// "closed:<name>" — a specific closed testing track by exact name
	// (min 2 chars, letters/digits/hyphen; no spaces — form-factor names use
	// ':' and Play's own defaults are "alpha"/"beta", so
	// letters+hyphen+digit is the safe charset).
	name, ok := strings.CutPrefix(channel, "closed:")
	if ok && closedTrackNameRx.MatchString(name) {
		return name, true
	}
	return "", false
}

// closedDefaultTrack is the closed testing track the hub creates and uses
// for channel "closed". Named "beta-testers" to stay distinct from Google's
// reserved "beta" (the open-testing track id).
const closedDefaultTrack = "beta-testers"

var closedTrackNameRx = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{1,28}$`)

// trackIsClosed reports whether a hub channel targets a closed testing
// track ("closed" or "closed:<name>") — the only tracks where pushing
// tester groups via the API is accepted.
func trackIsClosed(channel string) bool {
	_, ok := trackFor(channel)
	return ok && (channel == "closed" || strings.HasPrefix(channel, "closed:"))
}

// Publish uploads an AAB and assigns it to the Play track for the channel.
// Closed channels auto-create their track when missing (ensureClosedTrack).
// Tester groups (Google Group addresses) can be attached in the same edit
// for closed testing tracks. Returns the Play release name.
func (p *PlayPublisher) Publish(ctx context.Context, aabPath, channel, versionName, notes string, testers []string) (string, error) {
	track, ok := trackFor(channel)
	if !ok {
		return "", fmt.Errorf("play: channel %q has no Play track", channel)
	}
	if !strings.HasSuffix(strings.ToLower(aabPath), ".aab") {
		return "", fmt.Errorf("play: artifact must be an .aab (got %s)", aabPath)
	}
	if trackIsClosed(channel) {
		var err error
		if track, err = p.ensureClosedTrack(ctx, channel); err != nil {
			return "", err
		}
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
	// Tester groups (Google Group addresses) are only pushed on closed testing
	// tracks (closed:<name>), where the API accepts them. Google rejects group
	// updates on the internal track ("Cannot set tester group on an internal
	// track") — and on production/open the tester list is managed in Play
	// Console (open testing is open to everyone) — pushing groups there aborts
	// the whole release edit at commit.
	// Guarded on len>0: Testers.Update REPLACES the whole list, so pushing an
	// empty one would wipe testers added directly in Play Console.
	if trackIsClosed(channel) && len(testers) > 0 {
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

// ensureClosedTrack returns the Play track identifier for a hub closed
// channel, creating the track first when it doesn't exist yet. Play's
// edits.tracks.create supports exactly one type — closedTesting — which is
// precisely what we need. Harmless no-op when the track already exists
// (the create call is skipped).
func (p *PlayPublisher) ensureClosedTrack(ctx context.Context, channel string) (string, error) {
	track, ok := trackFor(channel)
	if !ok {
		return "", fmt.Errorf("play: channel %q is not a closed testing channel", channel)
	}
	existing, err := p.ListTracks(ctx)
	if err != nil {
		return "", err
	}
	for _, t := range existing {
		if t.Track == track {
			return track, nil
		}
	}
	appEdit, err := p.svc.Edits.Insert(p.pkgName, &androidpublisher.AppEdit{}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("play: open edit: %w", classifyPlayError(err))
	}
	defer func() {
		_ = p.svc.Edits.Delete(p.pkgName, appEdit.Id).Context(ctx).Do()
	}()
	if _, err := p.svc.Edits.Tracks.
		Create(p.pkgName, appEdit.Id, &androidpublisher.TrackConfig{
			Track: track,
			Type:  "closedTesting",
		}).
		Context(ctx).
		Do(); err != nil {
		// Lost a race with a concurrent create? Re-list; if it's there now,
		// fine.
		if existing2, err2 := p.ListTracks(ctx); err2 == nil {
			for _, t := range existing2 {
				if t.Track == track {
					return track, nil
				}
			}
		}
		return "", fmt.Errorf("play: create track %s: %w", track, classifyPlayError(err))
	}
	if _, err := p.svc.Edits.Commit(p.pkgName, appEdit.Id).Context(ctx).Do(); err != nil {
		return "", fmt.Errorf("play: commit track create: %w", err)
	}
	slog.Info("play: created closed testing track", "pkg", p.pkgName, "track", track)
	return track, nil
}

// ListTracks returns the app's existing tracks (name + releases) so the
// UI/API can show which closed testing tracks exist. Read-only and cheap —
// it opens and discards an edit, like Preflight.
func (p *PlayPublisher) ListTracks(ctx context.Context) ([]*androidpublisher.Track, error) {
	appEdit, err := p.svc.Edits.Insert(p.pkgName, &androidpublisher.AppEdit{}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("play: open edit: %w", classifyPlayError(err))
	}
	defer func() {
		_ = p.svc.Edits.Delete(p.pkgName, appEdit.Id).Context(ctx).Do()
	}()
	resp, err := p.svc.Edits.Tracks.List(p.pkgName, appEdit.Id).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("play: list tracks: %w", classifyPlayError(err))
	}
	return resp.Tracks, nil
}

// SetTesters replaces the tester list (Google Group addresses) on a Play
// track without touching releases — the manual "invite testers" action.
// The API cannot invite individual emails, only Google Groups; groups can
// be created free at groups.google.com. Only closed testing tracks accept
// group updates; the caller validates the track name.
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
