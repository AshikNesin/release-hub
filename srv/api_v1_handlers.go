package srv

// /api/v1 handlers that have no bearer-API twin: the app list and detail
// views, retention, hub settings, token and Play-account inventory.
// Wire types mirror web/src/lib/types.ts — change both together.

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
)

type v1Release struct {
	VersionCode int64  `json:"versionCode"`
	VersionName string `json:"versionName"`
	Channel     string `json:"channel"`
	Notes       string `json:"notes"`
	URL         string `json:"url"`
	SizeBytes   int64  `json:"sizeBytes"`
	Sha256      string `json:"sha256"`
	CreatedAt   string `json:"createdAt"`
	PlayStatus  string `json:"playStatus"` // pending | ok | error
	PlayError   string `json:"playError,omitempty"`
	PlayRelease string `json:"playRelease,omitempty"`
}

func toV1Release(slug, platform string, rel dbgen.Release) v1Release {
	return v1Release{
		VersionCode: rel.VersionCode, VersionName: rel.VersionName,
		Channel: rel.Channel, Notes: rel.Notes,
		URL:       "/artifacts/" + slug + "/" + platform + "/" + rel.FileName,
		SizeBytes: rel.SizeBytes, Sha256: rel.Sha256,
		CreatedAt:  rel.CreatedAt.UTC().Format(time.RFC3339),
		PlayStatus: rel.PlayStatus, PlayError: rel.PlayError, PlayRelease: rel.PlayRelease,
	}
}

// handleV1ListApps GET /api/v1/apps — every app with its platforms and the
// newest release of each.
func (s *Server) handleV1ListApps(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	apps, err := q.ListApps(r.Context())
	if err != nil {
		writeErr(w, 500, "db error")
		return
	}
	type platOut struct {
		Platform      string     `json:"platform"`
		PackageName   string     `json:"packageName"`
		PlayEnabled   bool       `json:"playEnabled"`
		HasSigningKey bool       `json:"hasSigningKey"`
		ReleaseCount  int        `json:"releaseCount"`
		Latest        *v1Release `json:"latest"`
	}
	type appOut struct {
		Slug      string    `json:"slug"`
		CreatedAt string    `json:"createdAt"`
		Platforms []platOut `json:"platforms"`
	}
	out := make([]appOut, 0, len(apps))
	for _, a := range apps {
		plats, err := q.ListPlatformsByApp(r.Context(), a.ID)
		if err != nil {
			writeErr(w, 500, "db error")
			return
		}
		ao := appOut{Slug: a.Slug, CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339), Platforms: []platOut{}}
		for _, p := range plats {
			rels, err := q.ListReleases(r.Context(), p.ID)
			if err != nil {
				writeErr(w, 500, "db error")
				return
			}
			po := platOut{
				Platform: p.Platform, PackageName: p.PackageName,
				PlayEnabled: p.PlayEnabled == 1, HasSigningKey: p.SignSha256 != "",
				ReleaseCount: len(rels),
			}
			if len(rels) > 0 {
				latest := toV1Release(a.Slug, p.Platform, rels[0])
				po.Latest = &latest
			}
			ao.Platforms = append(ao.Platforms, po)
		}
		out = append(out, ao)
	}
	writeJSON(w, 200, out)
}

// handleV1AppDetail GET /api/v1/apps/{slug} — everything the app page shows,
// per platform: releases (Play status reconciled lazily), share links,
// testers, retention, signing fingerprint and Play wiring.
func (s *Server) handleV1AppDetail(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	q := dbgen.New(s.DB)
	plats, err := q.ListPlatformsByApp(r.Context(), app.ID)
	if err != nil {
		writeErr(w, 500, "db error")
		return
	}
	type share struct {
		Label string `json:"label"`
		URL   string `json:"url"`
	}
	type retention struct {
		Override  *int64 `json:"override"`  // null = inherit the hub setting
		Effective int64  `json:"effective"` // what applies; 0 = keep everything
		Label     string `json:"label"`
	}
	type signing struct {
		HasKey bool   `json:"hasKey"`
		Sha256 string `json:"sha256"`
		Alias  string `json:"alias"`
	}
	type play struct {
		Enabled   bool   `json:"enabled"`
		AccountID int64  `json:"accountId"`
		Email     string `json:"email"`
	}
	type platOut struct {
		Platform     string      `json:"platform"`
		PackageName  string      `json:"packageName"`
		ManifestURL  string      `json:"manifestUrl"`
		UploadURL    string      `json:"uploadUrl"`
		Shares       []share     `json:"shares"`
		BetaTesters  []string    `json:"betaTesters"`
		TesterEmails []string    `json:"testerEmails"`
		Retention    retention   `json:"retention"`
		Signing      signing     `json:"signing"`
		Play         play        `json:"play"`
		Releases     []v1Release `json:"releases"`
	}
	type account struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
	}

	nonNil := func(v []string) []string {
		if v == nil {
			return []string{}
		}
		return v
	}
	betaTesters := nonNil(s.playTesters(r.Context()))
	testerEmails := nonNil(s.testerEmails(r.Context()))
	sections := make([]platOut, 0, len(plats))
	for _, p := range plats {
		releases, err := q.ListReleases(r.Context(), p.ID)
		if err != nil {
			writeErr(w, 500, "db error")
			return
		}
		if p.PlayEnabled == 1 {
			s.settlePlayStatus(r, p, releases)
		}
		rels := make([]v1Release, 0, len(releases))
		for _, rel := range releases {
			rels = append(rels, toV1Release(app.Slug, p.Platform, rel))
		}

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

		effective := int64(s.pruneConfig(r.Context(), p))
		dl := s.baseURL + "/apps/" + app.Slug + "/download?channel="
		shares := []share{{"direct", dl + "direct"}}
		if p.PlayEnabled == 1 {
			shares = append(shares, share{"internal", dl + "internal"}, share{"beta", dl + "beta"}, share{"public", dl + "public"})
		}
		sections = append(sections, platOut{
			Platform: p.Platform, PackageName: p.PackageName,
			ManifestURL:  s.baseURL + "/api/apps/" + app.Slug + "/" + p.Platform + "/manifest",
			UploadURL:    s.baseURL + "/api/apps/" + app.Slug + "/" + p.Platform + "/releases",
			Shares:       shares,
			BetaTesters:  betaTesters,
			TesterEmails: testerEmails,
			Retention:    retention{Override: p.PruneKeep, Effective: effective, Label: inheritLabel(&effective)},
			Signing:      signing{HasKey: p.SignSha256 != "", Sha256: p.SignSha256, Alias: alias},
			Play:         play{Enabled: p.PlayEnabled == 1, AccountID: acctID, Email: acctEmail},
			Releases:     rels,
		})
	}
	accounts := make([]account, 0)
	for _, acct := range s.mustListPlayAccounts(r) {
		accounts = append(accounts, account{acct.ID, playEmail(acct.Credentials)})
	}
	writeJSON(w, 200, map[string]any{
		"slug":             app.Slug,
		"createdAt":        app.CreatedAt.UTC().Format(time.RFC3339),
		"suggestedPackage": suggestPackage(s.bundlePrefix(r.Context()), app.Slug),
		"platforms":        sections,
		"playAccounts":     accounts,
	})
}

// handleV1Retention PUT /api/v1/apps/{slug}/{platform}/retention {keep}
// keep: "" / null = inherit the hub setting, 0 = keep everything, N = newest N.
func (s *Server) handleV1Retention(w http.ResponseWriter, r *http.Request) {
	plat, ok := s.platformFromRequest(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	keep, err := parseKeep(r.FormValue("keep"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := dbgen.New(s.DB).SetPruneKeep(r.Context(), dbgen.SetPruneKeepParams{PruneKeep: keep, ID: plat.ID}); err != nil {
		writeErr(w, 500, "save failed: "+err.Error())
		return
	}
	plat.PruneKeep = keep
	effective := int64(s.pruneConfig(r.Context(), plat))
	writeJSON(w, 200, map[string]any{"override": keep, "effective": effective, "label": inheritLabel(&effective)})
}

// ---- hub settings ----

type v1CertField struct {
	Field       string `json:"field"`
	Label       string `json:"label"`
	Hint        string `json:"hint"`
	Placeholder string `json:"placeholder"`
	Value       string `json:"value"`
}

// handleV1GetSettings GET /api/v1/settings
func (s *Server) handleV1GetSettings(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	get := func(key string) string {
		v, _ := q.GetConfig(r.Context(), key)
		return v
	}
	fields := make([]v1CertField, 0, len(signingSettingsFields))
	for _, f := range signingSettingsFields {
		fields = append(fields, v1CertField{f.Field, f.Label, f.Hint, f.Placeholder, get(f.Key)})
	}
	writeJSON(w, 200, map[string]any{
		"bundlePrefix":     s.bundlePrefix(r.Context()),
		"playTesters":      get("play_testers"),
		"playTesterEmails": get("play_tester_emails"),
		"pruneKeep":        get("prune_keep"),
		"certFields":       fields,
	})
}

// handleV1PatchSettings PATCH /api/v1/settings — only the keys present in
// the body are written; an absent key means "not part of this save".
// Keys: bundle_prefix, play_testers, play_tester_emails, prune_keep and the
// cert-subject fields (org, ou, locality, state, country).
func (s *Server) handleV1PatchSettings(w http.ResponseWriter, r *http.Request) {
	if status, err := s.saveSettings(r.Context(), r.PostForm); err != nil {
		writeErr(w, status, err.Error())
		return
	}
	s.handleV1GetSettings(w, r)
}

type settingsError string

func (e settingsError) Error() string { return string(e) }

func (s *Server) saveSettings(ctx context.Context, posted map[string][]string) (int, error) {
	q := dbgen.New(s.DB)
	has := func(k string) bool { _, ok := posted[k]; return ok }
	val := func(k string) string {
		if v := posted[k]; len(v) > 0 {
			return strings.TrimSpace(v[0])
		}
		return ""
	}
	// Validate before writing anything, so a bad value saves nothing.
	if has("prune_keep") && val("prune_keep") != "" {
		if n, err := strconv.Atoi(val("prune_keep")); err != nil || n < 0 {
			return 400, settingsError("release retention must be a number ≥ 0")
		}
	}
	if has("country") && val("country") != "" && len(val("country")) != 2 {
		return 400, settingsError("country must be a two-letter code, e.g. IN")
	}
	set := func(key, value string) error {
		return q.SetConfig(ctx, dbgen.SetConfigParams{Key: key, Value: value})
	}
	writes := map[string]string{}
	for _, f := range signingSettingsFields {
		if has(f.Field) {
			writes[f.Key] = val(f.Field)
		}
	}
	if has("bundle_prefix") {
		writes["bundle_prefix"] = strings.Trim(val("bundle_prefix"), ".")
	}
	if has("play_testers") {
		writes["play_testers"] = strings.Join(normalizeTesterGroups(val("play_testers")), ", ")
	}
	if has("play_tester_emails") {
		writes["play_tester_emails"] = strings.Join(normalizeTesterGroups(val("play_tester_emails")), ", ")
	}
	if has("prune_keep") {
		writes["prune_keep"] = val("prune_keep")
	}
	for k, v := range writes {
		if err := set(k, v); err != nil {
			return 500, settingsError("save failed: " + err.Error())
		}
	}
	return 200, nil
}

// ---- tokens ----

// handleV1ListTokens GET /api/v1/tokens — names and usage, never the secret.
func (s *Server) handleV1ListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := dbgen.New(s.DB).ListApiTokens(r.Context())
	if err != nil {
		writeErr(w, 500, "db error")
		return
	}
	type tokOut struct {
		ID         int64   `json:"id"`
		Name       string  `json:"name"`
		CreatedAt  string  `json:"createdAt"`
		LastUsedAt *string `json:"lastUsedAt"`
	}
	out := make([]tokOut, 0, len(tokens))
	for _, t := range tokens {
		row := tokOut{ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339)}
		if t.LastUsedAt != nil {
			v := t.LastUsedAt.UTC().Format(time.RFC3339)
			row.LastUsedAt = &v
		}
		out = append(out, row)
	}
	writeJSON(w, 200, out)
}

// handleV1DeleteToken DELETE /api/v1/tokens/{id} — revoke immediately.
func (s *Server) handleV1DeleteToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid token id")
		return
	}
	if err := dbgen.New(s.DB).DeleteApiToken(r.Context(), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"deleted": true})
}

// ---- shared Play service accounts ----

// handleV1ListPlayAccounts GET /api/v1/play-accounts — ids, emails and which
// app platforms publish through each.
func (s *Server) handleV1ListPlayAccounts(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	slugByID := map[int64]string{}
	if apps, err := q.ListApps(r.Context()); err == nil {
		for _, a := range apps {
			slugByID[a.ID] = a.Slug
		}
	}
	plats, _ := q.ListPlatformsWithPlay(r.Context())
	type row struct {
		ID        int64    `json:"id"`
		Email     string   `json:"email"`
		CreatedAt string   `json:"createdAt"`
		UsedBy    []string `json:"usedBy"` // "slug/platform"
	}
	out := make([]row, 0)
	for _, acct := range s.mustListPlayAccounts(r) {
		rw := row{ID: acct.ID, Email: playEmail(acct.Credentials), CreatedAt: acct.CreatedAt.UTC().Format(time.RFC3339), UsedBy: []string{}}
		for _, p := range plats {
			if p.PlayAccountID != nil && *p.PlayAccountID == acct.ID && p.PlayEnabled == 1 {
				rw.UsedBy = append(rw.UsedBy, slugByID[p.AppID]+"/"+p.Platform)
			}
		}
		out = append(out, rw)
	}
	writeJSON(w, 200, out)
}

// handleV1DeletePlayAccount DELETE /api/v1/play-accounts/{id}
func (s *Server) handleV1DeletePlayAccount(w http.ResponseWriter, r *http.Request) {
	if r.Form == nil {
		r.Form = map[string][]string{}
	}
	r.Form.Set("id", r.PathValue("id"))
	s.handleApiDeletePlayAccount(w, r)
}
