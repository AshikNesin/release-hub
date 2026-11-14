package srv

// Shared helpers behind the UI JSON API (api_v1*.go): Play links and
// identity, lazy Play-status reconciliation, settings metadata.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"srv.exe.dev/db/dbgen"
)

func (s *Server) hasPassword() (bool, error) {
	ctx := context.Background()
	_, err := dbgen.New(s.DB).GetConfig(ctx, "admin_password_hash")
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// playInternalTestingURL, playOpenTestingURL and playStoreURL are the
// user-facing links for a package on Google Play. internaltesting/ is the
// opt-in page testers use to join the internal-testing track; beta is the
// open-testing opt-in page (Play's track id for open testing); details is
// the public listing.
func playInternalTestingURL(pkg string) string {
	return "https://play.google.com/apps/testing/" + pkg
}

func playOpenTestingURL(pkg string) string {
	// Same opt-in URL as internal — Play resolves the tester's eligible
	// tracks (internal wins if joined; otherwise open) from the package.
	return "https://play.google.com/apps/testing/" + pkg
}

func playStoreURL(pkg string) string {
	return "https://play.google.com/store/apps/details?id=" + pkg
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

// settlePlayStatus reconciles 'pending' Play-channel releases against the
// app's live Play tracks: if the release's versionName appears on the track
// it targeted, mark ok; leave it pending otherwise (never guess error).
// In-memory only for rows still pending in DB; persists the outcome.
func (s *Server) settlePlayStatus(r *http.Request, plat dbgen.AppPlatform, releases []dbgen.Release) {
	need := false
	for _, rel := range releases {
		if rel.PlayStatus == "pending" {
			if _, ok := trackFor(rel.Channel); ok {
				need = true
				break
			}
		}
	}
	if !need || plat.PlayAccountID == nil {
		return
	}
	creds, err := s.playAccountCreds(*plat.PlayAccountID)
	if err != nil {
		return
	}
	pub, err := NewPlayPublisherFromJSON(r.Context(), plat.PackageName, creds)
	if err != nil {
		return
	}
	tracks, err := pub.ListTracks(r.Context())
	if err != nil {
		slog.Debug("settle: list tracks failed", "error", err)
		return
	}
	// trackName → set of release names on it
	onPlay := map[string]map[string]bool{}
	for _, tr := range tracks {
		set := map[string]bool{}
		for _, rel := range tr.Releases {
			set[rel.Name] = true
		}
		onPlay[tr.Track] = set
	}
	q := dbgen.New(s.DB)
	for i := range releases {
		rel := &releases[i] // pointer: the caller renders the settled status right away
		if rel.PlayStatus != "pending" {
			continue
		}
		track, ok := trackFor(rel.Channel)
		if !ok {
			continue
		}
		// Play release names are "versionName (bundleVersionCode)".
		wantName := fmt.Sprintf("%s (%d)", rel.VersionName, rel.VersionCode)
		// Hub versionCode can trail the Play bundle code by the hub-only rows
		// recorded after failed pushes — match on either the name form or any
		// release on the track containing the versionName.
		if set := onPlay[track]; set != nil {
			found, matched := set[wantName], wantName
			if !found {
				for name := range set {
					if strings.HasPrefix(name, rel.VersionName+" (") {
						found, matched = true, name
						break
					}
				}
			}
			if found {
				if err := q.SetPlayStatus(r.Context(), dbgen.SetPlayStatusParams{
					PlayStatus: "ok", PlayError: "", PlayRelease: matched,
					AppPlatformID: plat.ID, VersionCode: rel.VersionCode,
				}); err == nil {
					rel.PlayStatus, rel.PlayRelease = "ok", matched
				}
			}
		}
	}
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
