package srv

// GitHub App client: app JWTs, installation tokens, the live repository list
// and repository Actions variables. The app itself is registered through
// GitHub's manifest flow (github_app.go); its private key is stored sealed
// or in 1Password like every other secret (secrets.go).
//
// Ported from FeedbackBee's apps/web/src/server/api/github/github.client.ts.
// Installation tokens live in memory only, are minted on demand and
// refreshed once on a 401; nothing GitHub-issued besides the app key is
// stored.

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"srv.exe.dev/db/dbgen"
)

const (
	defaultGithubAPI = "https://api.github.com"
	defaultGithubWeb = "https://github.com"

	// The two repo Actions variables Connect sets.
	varReleaseHubURL = "RELEASE_HUB_URL"
	varReleaseHubApp = "RELEASE_HUB_APP"
)

// githubError is a GitHub API failure with a message safe to show.
type githubError struct {
	Status int
	Msg    string
}

func (e *githubError) Error() string { return e.Msg }

// errNoGithubApp: no app registered yet.
var errNoGithubApp = errors.New("connect a GitHub App in Settings → GitHub first")

type githubTokenCache struct {
	mu     sync.Mutex
	tokens map[string]cachedGithubToken
}

type cachedGithubToken struct {
	token   string
	expires time.Time
}

func (c *githubTokenCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.tokens[key]
	if !ok || time.Until(t.expires) < time.Minute {
		return "", false
	}
	return t.token, true
}

func (c *githubTokenCache) put(key, token string, expires time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tokens == nil || len(c.tokens) > 200 {
		c.tokens = map[string]cachedGithubToken{}
	}
	c.tokens[key] = cachedGithubToken{token: token, expires: expires}
}

func (c *githubTokenCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens = nil
}

// githubDo calls the GitHub REST API. token may be empty (manifest
// conversion), an app JWT or an installation token. Redirects are never
// followed, so credentials can't leak to another host.
func (s *Server) githubDo(ctx context.Context, method, path, token string, body any) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.githubAPI+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "release-hub")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.githubHTTP.Do(req)
	if err != nil {
		return nil, &githubError{Status: http.StatusBadGateway, Msg: "GitHub is unreachable. Try again later"}
	}
	return resp, nil
}

// githubJSON calls the API and decodes a 2xx JSON reply into out (nil to
// discard). Any other status becomes a githubError carrying GitHub's message.
func (s *Server) githubJSON(ctx context.Context, method, path, token string, body, out any) error {
	resp, err := s.githubDo(ctx, method, path, token, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeGithub(resp, method, path, out)
}

func decodeGithub(resp *http.Response, method, path string, out any) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		msg := fmt.Sprintf("GitHub %d on %s %s", resp.StatusCode, method, strings.SplitN(path, "?", 2)[0])
		if e.Message != "" {
			msg += ": " + e.Message
		}
		return &githubError{Status: resp.StatusCode, Msg: msg}
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &githubError{Status: http.StatusBadGateway, Msg: "GitHub returned an invalid response"}
	}
	return nil
}

// parseAppKey parses a GitHub App private key (PKCS#1, as GitHub issues
// them, or PKCS#8).
func parseAppKey(pemText []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemText)
	if block == nil {
		return nil, errors.New("GitHub App private key is not PEM")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("GitHub App private key: %w", err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("GitHub App private key is not RSA")
	}
	return rk, nil
}

// signAppJWT mints the 10-minute JWT that authenticates as the app itself
// (iat backdated a minute for clock drift, as GitHub recommends).
func signAppJWT(appID int64, key crypto.Signer) (string, error) {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return "", err
	}
	now := time.Now()
	return jwt.Signed(signer).Claims(jwt.Claims{
		Issuer:   strconv.FormatInt(appID, 10),
		IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)),
		Expiry:   jwt.NewNumericDate(now.Add(9 * time.Minute)),
	}).Serialize()
}

type githubAuth struct {
	app dbgen.GithubApp
	jwt string
}

// githubAppAuth loads the registered app and signs an app JWT.
func (s *Server) githubAppAuth(ctx context.Context) (githubAuth, error) {
	app, err := dbgen.New(s.DB).GetGithubApp(ctx)
	if err != nil {
		return githubAuth{}, errNoGithubApp
	}
	pemText, err := s.openSecret(ctx, app.PrivateKey)
	if err != nil {
		return githubAuth{}, fmt.Errorf("GitHub App private key: %w", err)
	}
	key, err := parseAppKey(pemText)
	if err != nil {
		return githubAuth{}, err
	}
	tok, err := signAppJWT(app.AppID, key)
	if err != nil {
		return githubAuth{}, err
	}
	return githubAuth{app: app, jwt: tok}, nil
}

// installationToken mints (or reuses) an installation token. With a
// repository id it is scoped down to that repo and to the permissions
// Connect needs; without, it lists the installation's repos.
func (s *Server) installationToken(ctx context.Context, auth githubAuth, installationID, repositoryID int64, refresh bool) (string, error) {
	key := fmt.Sprintf("%d:%d:%d", auth.app.AppID, installationID, repositoryID)
	if !refresh {
		if t, ok := s.githubTokens.get(key); ok {
			return t, nil
		}
	}
	body := map[string]any{"permissions": map[string]string{"metadata": "read", "actions_variables": "write"}}
	if repositoryID > 0 {
		body["repository_ids"] = []int64{repositoryID}
	}
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := s.githubJSON(ctx, http.MethodPost, fmt.Sprintf("/app/installations/%d/access_tokens", installationID), auth.jwt, body, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", &githubError{Status: http.StatusBadGateway, Msg: "GitHub returned no installation token"}
	}
	s.githubTokens.put(key, out.Token, out.ExpiresAt)
	return out.Token, nil
}

type githubInstallation struct {
	ID          int64             `json:"id"`
	SuspendedAt *string           `json:"suspended_at"`
	Permissions map[string]string `json:"permissions"`
	Account     struct {
		Login string `json:"login"`
	} `json:"account"`
}

// usable: not suspended, and granted the Variables permission Connect needs.
func (i githubInstallation) usable() bool {
	return i.SuspendedAt == nil && i.Permissions["actions_variables"] == "write"
}

type githubRepo struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Owner         struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"owner"`
}

// githubRepository is a repo the app can reach, as the UI lists it.
type githubRepository struct {
	ID             int64  `json:"id"`
	FullName       string `json:"fullName"`
	OwnerID        int64  `json:"ownerId"`
	DefaultBranch  string `json:"defaultBranch"`
	Private        bool   `json:"private"`
	InstallationID int64  `json:"installationId"`
}

// listGithubRepositories reads every repo of every usable installation,
// live from GitHub, never from a local allow-list that could outlive
// revoked access.
func (s *Server) listGithubRepositories(ctx context.Context) ([]githubRepository, error) {
	auth, err := s.githubAppAuth(ctx)
	if err != nil {
		return nil, err
	}
	out := []githubRepository{}
	for page := 1; ; page++ {
		var insts []githubInstallation
		if err := s.githubJSON(ctx, http.MethodGet, fmt.Sprintf("/app/installations?per_page=100&page=%d", page), auth.jwt, nil, &insts); err != nil {
			return nil, err
		}
		for _, inst := range insts {
			if !inst.usable() {
				continue
			}
			repos, err := s.installationRepos(ctx, auth, inst.ID)
			if err != nil {
				return nil, err
			}
			for _, r := range repos {
				out = append(out, githubRepository{
					ID: r.ID, FullName: r.FullName, OwnerID: r.Owner.ID,
					DefaultBranch: r.DefaultBranch, Private: r.Private, InstallationID: inst.ID,
				})
			}
		}
		if len(insts) < 100 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].FullName) < strings.ToLower(out[j].FullName) })
	return out, nil
}

func (s *Server) installationRepos(ctx context.Context, auth githubAuth, installationID int64) ([]githubRepo, error) {
	var all []githubRepo
	for page := 1; ; page++ {
		var body struct {
			Repositories []githubRepo `json:"repositories"`
		}
		path := fmt.Sprintf("/installation/repositories?per_page=100&page=%d", page)
		if err := s.withInstallationToken(ctx, auth, installationID, 0, func(tok string) error {
			return s.githubJSON(ctx, http.MethodGet, path, tok, nil, &body)
		}); err != nil {
			return nil, err
		}
		all = append(all, body.Repositories...)
		if len(body.Repositories) < 100 {
			return all, nil
		}
	}
}

// withInstallationToken runs call with an installation token, retrying once
// with a fresh token when GitHub says the cached one is no longer valid.
func (s *Server) withInstallationToken(ctx context.Context, auth githubAuth, installationID, repositoryID int64, call func(string) error) error {
	tok, err := s.installationToken(ctx, auth, installationID, repositoryID, false)
	if err != nil {
		return err
	}
	err = call(tok)
	var ge *githubError
	if errors.As(err, &ge) && ge.Status == http.StatusUnauthorized {
		if tok, err = s.installationToken(ctx, auth, installationID, repositoryID, true); err != nil {
			return err
		}
		err = call(tok)
	}
	return err
}

// githubRepoClient talks to one repository with a repo-scoped installation
// token.
type githubRepoClient struct {
	s              *Server
	auth           githubAuth
	installationID int64
	repo           githubRepo
}

// githubRepoAccess resolves "owner/name" to its installation and metadata
// (ids, default branch). The installation must be usable.
func (s *Server) githubRepoAccess(ctx context.Context, fullName string) (*githubRepoClient, error) {
	owner, name, ok := strings.Cut(strings.TrimSpace(fullName), "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return nil, &githubError{Status: http.StatusBadRequest, Msg: "repository must be owner/name"}
	}
	auth, err := s.githubAppAuth(ctx)
	if err != nil {
		return nil, err
	}
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
	var inst githubInstallation
	if err := s.githubJSON(ctx, http.MethodGet, path+"/installation", auth.jwt, nil, &inst); err != nil {
		var ge *githubError
		if errors.As(err, &ge) && ge.Status == http.StatusNotFound {
			return nil, &githubError{Status: http.StatusBadRequest, Msg: "the GitHub App is not installed on " + fullName + ". Install it on that repository first"}
		}
		return nil, err
	}
	if !inst.usable() {
		return nil, &githubError{Status: http.StatusBadRequest, Msg: "the GitHub App installation for " + fullName + " is suspended or lacks the Variables: read and write permission. Accept the new permissions on GitHub, or reinstall"}
	}
	c := &githubRepoClient{s: s, auth: auth, installationID: inst.ID}
	if err := s.withInstallationToken(ctx, auth, inst.ID, 0, func(tok string) error {
		return s.githubJSON(ctx, http.MethodGet, path, tok, nil, &c.repo)
	}); err != nil {
		return nil, err
	}
	if c.repo.ID == 0 || c.repo.Owner.ID == 0 {
		return nil, &githubError{Status: http.StatusBadGateway, Msg: "GitHub returned an invalid repository"}
	}
	return c, nil
}

// githubRepoAccessByID finds a repository the app can reach by its id (it
// may have been renamed since the grant was made).
func (s *Server) githubRepoAccessByID(ctx context.Context, id int64) (*githubRepoClient, error) {
	repos, err := s.listGithubRepositories(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range repos {
		if r.ID == id {
			return s.githubRepoAccess(ctx, r.FullName)
		}
	}
	return nil, &githubError{Status: http.StatusNotFound, Msg: "the GitHub App can no longer reach that repository"}
}

func (c *githubRepoClient) call(ctx context.Context, method, path string, body, out any) error {
	return c.s.withInstallationToken(ctx, c.auth, c.installationID, c.repo.ID, func(tok string) error {
		return c.s.githubJSON(ctx, method, path, tok, body, out)
	})
}

func (c *githubRepoClient) varsPath() string {
	owner, name, _ := strings.Cut(c.repo.FullName, "/")
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name) + "/actions/variables"
}

// variable returns a repo Actions variable's value; ok=false when unset.
func (c *githubRepoClient) variable(ctx context.Context, name string) (value string, ok bool, err error) {
	var v struct {
		Value string `json:"value"`
	}
	err = c.call(ctx, http.MethodGet, c.varsPath()+"/"+url.PathEscape(name), nil, &v)
	var ge *githubError
	if errors.As(err, &ge) && ge.Status == http.StatusNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v.Value, true, nil
}

// setVariable creates or updates a repo Actions variable.
func (c *githubRepoClient) setVariable(ctx context.Context, name, value string, exists bool) error {
	body := map[string]string{"name": name, "value": value}
	if exists {
		return c.call(ctx, http.MethodPatch, c.varsPath()+"/"+url.PathEscape(name), body, nil)
	}
	return c.call(ctx, http.MethodPost, c.varsPath(), body, nil)
}

// deleteVariable removes a repo Actions variable; already gone is fine.
func (c *githubRepoClient) deleteVariable(ctx context.Context, name string) error {
	err := c.call(ctx, http.MethodDelete, c.varsPath()+"/"+url.PathEscape(name), nil, nil)
	var ge *githubError
	if errors.As(err, &ge) && ge.Status == http.StatusNotFound {
		return nil
	}
	return err
}
