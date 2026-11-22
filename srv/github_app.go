package srv

// One-click keyless publishing through this release hub's own GitHub App
// (issue #8). The flow is ported from FeedbackBee
// (apps/web/src/server/api/github/github.service.ts, github.routes.ts):
//
//  1. POST /api/v1/github/manifest returns GitHub's "new app" URL and a
//     manifest; the browser auto-submits it as a form. GitHub shows one
//     confirm screen and redirects to GET /api/v1/github/callback?code&state.
//     The code is converted (POST /app-manifests/{code}/conversions) into
//     the app's id, slug and private key; the key is sealed or kept in
//     1Password (sealSecrets), never shown or pasted.
//  2. POST /api/v1/github/install sends the browser to the app's install
//     page, where the user picks repositories; GitHub returns to
//     GET /api/v1/github/installed?installation_id&state.
//  3. POST /api/v1/apps/{slug}/github/connect binds a picked repository to
//     the app: a repository OIDC grant (by id, default branch) plus the repo
//     Actions variables RELEASE_HUB_URL and RELEASE_HUB_APP.
//
// Each round trip to github.com is a flow: a random state (travels through
// GitHub) and a random binding (an httpOnly cookie on this browser only),
// both stored hashed, single use, 10 minutes. Callbacks also need the admin
// session, which the Lax session cookie carries on GitHub's top-level
// redirect. Webhooks are off: GitHub never has to reach the release hub.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
)

const (
	githubFlowCookie = "rh_github_flow"
	githubFlowPath   = "/api/v1/github"
	githubFlowTTL    = 10 * time.Minute
)

var githubOrgRe = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$`)

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// githubBaseURL returns the release hub's public base URL when GitHub can
// use it: https, no credentials, query or fragment, not localhost. GitHub
// redirects the browser there and CI reads it from RELEASE_HUB_URL.
func (s *Server) githubBaseURL() (string, error) {
	base := strings.TrimRight(s.baseURL, "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || isLocalhost(u.Host) {
		return "", fmt.Errorf("set RELEASE_HUB_BASE_URL to this release hub's public https URL before connecting GitHub (it is %q)", s.baseURL)
	}
	return base, nil
}

// canStoreGithubKey: sealSecrets needs 1Password or a sealing key.
func (s *Server) canStoreGithubKey() bool {
	if s.vault != nil || envStr("ALLOW_PLAINTEXT_CREDS") == "1" {
		return true
	}
	k, err := loadSecretKey()
	return err == nil && k != nil
}

// writeGithubErr maps GitHub and setup errors onto API errors.
func writeGithubErr(w http.ResponseWriter, err error) {
	var ge *githubError
	switch {
	case errors.Is(err, errNoGithubApp):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.As(err, &ge):
		status := ge.Status
		if status < 400 || status == http.StatusUnauthorized || status >= 500 {
			status = http.StatusBadGateway // never sign the UI out over GitHub's 401
		}
		writeErr(w, status, ge.Msg)
	default:
		writeErr(w, http.StatusBadGateway, err.Error())
	}
}

// ---- flows ----

func (s *Server) beginGithubFlow(ctx context.Context, w http.ResponseWriter, kind string) (state string, err error) {
	q := dbgen.New(s.DB)
	now := time.Now()
	_ = q.DeleteExpiredGithubFlows(ctx, now)
	state, binding := randomHex(24), randomHex(24)
	if err := q.CreateGithubFlow(ctx, dbgen.CreateGithubFlowParams{
		State: hashToken(state), Binding: hashToken(binding), Kind: kind, ExpiresAt: now.Add(githubFlowTTL),
	}); err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name: githubFlowCookie, Value: binding, Path: githubFlowPath,
		HttpOnly: true, Secure: strings.HasPrefix(s.baseURL, "https://"),
		SameSite: http.SameSiteLaxMode, MaxAge: int(githubFlowTTL.Seconds()),
	})
	return state, nil
}

// claimGithubFlow consumes a flow: state from GitHub's redirect, binding
// from this browser's cookie. Each flow can be claimed once.
func (s *Server) claimGithubFlow(ctx context.Context, r *http.Request, kind string) error {
	state := r.URL.Query().Get("state")
	c, err := r.Cookie(githubFlowCookie)
	if err != nil || c.Value == "" || len(state) < 32 || len(state) > 100 {
		return errors.New("missing or invalid GitHub setup state")
	}
	n, err := dbgen.New(s.DB).ClaimGithubFlow(ctx, dbgen.ClaimGithubFlowParams{
		State: hashToken(state), Binding: hashToken(c.Value), Kind: kind, ExpiresAt: time.Now(),
	})
	if err != nil || n != 1 {
		return errors.New("GitHub setup expired or is invalid. Start again from Settings → GitHub")
	}
	return nil
}

// ---- settings ----

type githubAppInfo struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Owner      string `json:"owner"`
	HTMLURL    string `json:"htmlUrl"`
	InstallURL string `json:"installUrl"`
	CreatedAt  string `json:"createdAt"`
}

// handleV1GithubSettings GET /api/v1/github
func (s *Server) handleV1GithubSettings(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"canStoreKey": s.canStoreGithubKey(),
		"baseUrl":     s.baseURL,
		"baseUrlOk":   true,
		"app":         nil,
		"variables":   []string{varReleaseHubURL, varReleaseHubApp},
	}
	if _, err := s.githubBaseURL(); err != nil {
		out["baseUrlOk"] = false
		out["baseUrlProblem"] = err.Error()
	}
	if app, err := dbgen.New(s.DB).GetGithubApp(r.Context()); err == nil {
		out["app"] = githubAppInfo{
			Slug: app.Slug, Name: app.Name, Owner: app.Owner, HTMLURL: app.HtmlUrl,
			InstallURL: s.githubWeb + "/apps/" + url.PathEscape(app.Slug) + "/installations/new",
			CreatedAt:  app.CreatedAt.UTC().Format(time.RFC3339),
		}
	}
	writeJSON(w, 200, out)
}

// handleV1GithubRepositories GET /api/v1/github/repositories: every repo
// the app's installations can reach, with the app each is granted to.
func (s *Server) handleV1GithubRepositories(w http.ResponseWriter, r *http.Request) {
	repos, err := s.listGithubRepositories(r.Context())
	if err != nil {
		writeGithubErr(w, err)
		return
	}
	grants, err := dbgen.New(s.DB).ListAllOidcGrants(r.Context())
	if err != nil {
		writeErr(w, 500, "db error")
		return
	}
	grantedTo := map[int64]string{}
	for _, g := range grants {
		if g.RepositoryID != nil && g.Issuer == githubIssuer && g.AppSlug != nil {
			grantedTo[*g.RepositoryID] = *g.AppSlug
		}
	}
	type row struct {
		githubRepository
		App *string `json:"app"`
	}
	out := make([]row, 0, len(repos))
	for _, rp := range repos {
		rw := row{githubRepository: rp}
		if slug, ok := grantedTo[rp.ID]; ok {
			rw.App = &slug
		}
		out = append(out, rw)
	}
	writeJSON(w, 200, out)
}

// ---- registration (manifest flow) ----

// handleV1GithubManifest POST /api/v1/github/manifest {organization?}
func (s *Server) handleV1GithubManifest(w http.ResponseWriter, r *http.Request) {
	base, err := s.githubBaseURL()
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.canStoreGithubKey() {
		writeErr(w, 400, "set RELEASE_HUB_SECRET_KEY or configure 1Password before registering a GitHub App: its private key is stored like your signing keys")
		return
	}
	if _, err := dbgen.New(s.DB).GetGithubApp(r.Context()); err == nil {
		writeErr(w, 409, "disconnect the existing GitHub App before registering another")
		return
	}
	org := strings.TrimSpace(r.FormValue("organization"))
	if org != "" && !githubOrgRe.MatchString(org) {
		writeErr(w, 400, "organization must be a GitHub organization login")
		return
	}
	state, err := s.beginGithubFlow(r.Context(), w, "manifest")
	if err != nil {
		writeErr(w, 500, "db error")
		return
	}
	target := s.githubWeb + "/settings/apps/new"
	if org != "" {
		target = s.githubWeb + "/organizations/" + url.PathEscape(org) + "/settings/apps/new"
	}
	writeJSON(w, 200, map[string]any{
		"url": target + "?state=" + state,
		"manifest": map[string]any{
			// GitHub App names are global; a random suffix keeps every
			// self-hosted release hub's app distinct.
			"name":         "ReleaseHub-" + randomHex(4),
			"url":          base,
			"description":  "Keyless publishing for " + base + ": links repositories to apps and sets their RELEASE_HUB_URL / RELEASE_HUB_APP Actions variables.",
			"redirect_url": base + githubFlowPath + "/callback",
			"setup_url":    base + githubFlowPath + "/installed",
			"public":       false,
			"hook_attributes": map[string]any{
				"url":    base + githubFlowPath + "/webhook",
				"active": false,
			},
			"default_permissions": map[string]string{
				"metadata":          "read",
				"actions_variables": "write",
			},
			"default_events": []string{},
		},
	})
}

// githubCallback wraps the two browser redirects back from github.com: no
// envelope, always a redirect to Settings → GitHub with the outcome.
func (s *Server) githubCallback(kind string, complete func(*http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		http.SetCookie(w, &http.Cookie{Name: githubFlowCookie, Value: "", Path: githubFlowPath, MaxAge: -1})
		outcome := kind
		if !s.validSession(r) {
			outcome = "error"
		} else if err := s.claimGithubFlow(r.Context(), r, map[string]string{"connected": "manifest", "installed": "install"}[kind]); err != nil {
			slog.Warn("github setup rejected", "step", kind, "err", err.Error())
			outcome = "error"
		} else if err := complete(r); err != nil {
			slog.Warn("github setup failed", "step", kind, "err", err.Error())
			outcome = "error"
		}
		http.Redirect(w, r, "/settings/github?github="+outcome, http.StatusSeeOther)
	}
}

// completeGithubManifest converts the manifest code into the app's
// credentials and stores them (private key sealed / in 1Password).
func (s *Server) completeGithubManifest(r *http.Request) error {
	ctx := r.Context()
	code := r.URL.Query().Get("code")
	if code == "" || len(code) > 200 {
		return errors.New("missing manifest code")
	}
	q := dbgen.New(s.DB)
	if _, err := q.GetGithubApp(ctx); err == nil {
		return errors.New("a GitHub App is already connected")
	}
	var conv struct {
		ID      int64  `json:"id"`
		Slug    string `json:"slug"`
		Name    string `json:"name"`
		HTMLURL string `json:"html_url"`
		PEM     string `json:"pem"`
		Owner   struct {
			Login string `json:"login"`
		} `json:"owner"`
	}
	if err := s.githubJSON(ctx, http.MethodPost, "/app-manifests/"+url.PathEscape(code)+"/conversions", "", nil, &conv); err != nil {
		return err
	}
	if conv.ID <= 0 || conv.Slug == "" || conv.PEM == "" {
		return errors.New("GitHub returned incomplete app credentials")
	}
	if _, err := parseAppKey([]byte(conv.PEM)); err != nil {
		return err
	}
	refs, err := s.sealSecrets(ctx, "release-hub: GitHub App "+conv.Slug,
		"Private key of the GitHub App "+conv.Slug+" that release-hub uses to link repositories for keyless publishing. Regenerate it on GitHub if it leaks.",
		[]vaultValue{{ID: "private_key", Label: "private key (PEM)", Data: []byte(conv.PEM), Concealed: true}})
	if err != nil {
		return fmt.Errorf("store the GitHub App key: %w", err)
	}
	if err := q.CreateGithubApp(ctx, dbgen.CreateGithubAppParams{
		AppID: conv.ID, Slug: conv.Slug, Name: conv.Name, Owner: conv.Owner.Login,
		HtmlUrl: conv.HTMLURL, PrivateKey: refs[0],
	}); err != nil {
		s.retireSecrets(ctx, refs[0])
		return err
	}
	s.githubTokens.clear()
	return nil
}

// handleV1GithubInstall POST /api/v1/github/install → {url}
func (s *Server) handleV1GithubInstall(w http.ResponseWriter, r *http.Request) {
	app, err := dbgen.New(s.DB).GetGithubApp(r.Context())
	if err != nil {
		writeErr(w, 409, errNoGithubApp.Error())
		return
	}
	state, err := s.beginGithubFlow(r.Context(), w, "install")
	if err != nil {
		writeErr(w, 500, "db error")
		return
	}
	writeJSON(w, 200, map[string]string{
		"url": s.githubWeb + "/apps/" + url.PathEscape(app.Slug) + "/installations/new?state=" + state,
	})
}

// completeGithubInstall: the installation_id in the redirect is not
// authority. It must be one of this app's installations, usable, and
// reach at least one repository.
func (s *Server) completeGithubInstall(r *http.Request) error {
	id, err := strconv.ParseInt(r.URL.Query().Get("installation_id"), 10, 64)
	if err != nil || id <= 0 {
		// setup_action=request: an org admin still has to approve it.
		if r.URL.Query().Get("setup_action") == "request" {
			return nil
		}
		return errors.New("missing installation id")
	}
	repos, err := s.listGithubRepositories(r.Context())
	if err != nil {
		return err
	}
	for _, rp := range repos {
		if rp.InstallationID == id {
			return nil
		}
	}
	return errors.New("no accessible repositories found for this GitHub installation")
}

// handleV1GithubDisconnect POST /api/v1/github/disconnect: forgets the app
// locally. It stays registered and installed on GitHub; grants keep working
// (OIDC doesn't need the app).
func (s *Server) handleV1GithubDisconnect(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	app, err := q.GetGithubApp(r.Context())
	if err == nil {
		if err := q.DeleteGithubApp(r.Context()); err != nil {
			writeErr(w, 500, "db error")
			return
		}
		s.retireSecrets(r.Context(), app.PrivateKey)
	}
	_ = q.DeleteGithubFlows(r.Context())
	s.githubTokens.clear()
	writeJSON(w, 200, map[string]bool{"disconnected": true})
}

// ---- connect a repository to an app ----

type githubVariableState struct {
	Name    string  `json:"name"`
	Value   string  `json:"value"`             // what release-hub sets
	Current *string `json:"current,omitempty"` // what the repo has now, when different
}

// handleV1GithubConnect POST /api/v1/apps/{slug}/github/connect
// {repository: "owner/name", overwrite?: bool}
//
// Creates a repository grant (by id, for the default branch) and sets the
// repo's RELEASE_HUB_URL and RELEASE_HUB_APP variables. When a variable
// already holds a different value and overwrite is not set, nothing is
// changed and the reply lists the conflicts ({connected: false, conflicts}).
func (s *Server) handleV1GithubConnect(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	base, err := s.githubBaseURL()
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	rc, err := s.githubRepoAccess(ctx, r.FormValue("repository"))
	if err != nil {
		writeGithubErr(w, err)
		return
	}
	q := dbgen.New(s.DB)
	repoID := rc.repo.ID
	if g, err := q.OidcGrantByIssuerRepository(ctx, dbgen.OidcGrantByIssuerRepositoryParams{Issuer: githubIssuer, RepositoryID: &repoID}); err == nil {
		if g.AppID == app.ID {
			writeErr(w, 409, rc.repo.FullName+" is already connected to this app")
		} else {
			writeErr(w, 409, rc.repo.FullName+" already publishes "+g.AppSlug+" (one repo maps to one app). Disconnect it there first")
		}
		return
	}

	want := []githubVariableState{{Name: varReleaseHubURL, Value: base}, {Name: varReleaseHubApp, Value: app.Slug}}
	exists := make([]bool, len(want))
	var conflicts []githubVariableState
	for i := range want {
		cur, ok, err := rc.variable(ctx, want[i].Name)
		if err != nil {
			writeGithubErr(w, err)
			return
		}
		exists[i] = ok
		if ok && cur != want[i].Value {
			c := want[i]
			c.Current = &cur
			conflicts = append(conflicts, c)
		}
	}
	if len(conflicts) > 0 && r.FormValue("overwrite") != "true" {
		writeJSON(w, 200, map[string]any{"connected": false, "repository": rc.repo.FullName, "conflicts": conflicts})
		return
	}
	for i, v := range want {
		if err := rc.setVariable(ctx, v.Name, v.Value, exists[i]); err != nil {
			writeGithubErr(w, err)
			return
		}
	}

	ownerID := rc.repo.Owner.ID
	branch := rc.repo.DefaultBranch
	if branch == "" {
		branch = "main"
	}
	ref := "refs/heads/" + branch
	if _, err := q.CreateOidcRepositoryGrant(ctx, dbgen.CreateOidcRepositoryGrantParams{
		AppID: app.ID, Issuer: githubIssuer, Subject: repositorySubject(repoID), Name: rc.repo.FullName,
		RepositoryID: &repoID, RepositoryOwnerID: &ownerID, Ref: ref,
	}); err != nil {
		writeErr(w, 409, "a grant for "+rc.repo.FullName+" already exists (one repo maps to one app)")
		return
	}
	writeJSON(w, 200, map[string]any{
		"connected": true, "repository": rc.repo.FullName, "ref": ref, "variables": want,
	})
}

// repositorySubject is the synthetic, never-matched subject a repository
// grant stores (UNIQUE(issuer, subject) still has to hold).
func repositorySubject(id int64) string {
	return "github-repository:" + strconv.FormatInt(id, 10)
}

// removeGithubVariables deletes the variables Connect set on a repository
// grant's repo, only where they still hold release-hub's values.
func (s *Server) removeGithubVariables(ctx context.Context, g dbgen.OidcGrantByIDRow) error {
	if g.RepositoryID == nil {
		return nil
	}
	rc, err := s.githubRepoAccessByID(ctx, *g.RepositoryID)
	if err != nil {
		return err
	}
	ours := map[string]string{varReleaseHubApp: ""}
	if g.AppSlug != nil {
		ours[varReleaseHubApp] = *g.AppSlug
	}
	if base, err := s.githubBaseURL(); err == nil {
		ours[varReleaseHubURL] = base
	}
	for name, val := range ours {
		cur, ok, err := rc.variable(ctx, name)
		if err != nil {
			return err
		}
		if ok && val != "" && cur == val {
			if err := rc.deleteVariable(ctx, name); err != nil {
				return err
			}
		}
	}
	return nil
}
