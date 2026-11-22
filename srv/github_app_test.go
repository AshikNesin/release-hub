package srv

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// ---- repository grants (match on repository_id, not sub) ----

// repoToken mints a GitHub-shaped token: sub plus repository claims (as
// strings, the way GitHub sends them).
func (fi *fakeIssuer) repoToken(t *testing.T, sub, repoID, ownerID, ref string) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: fi.key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := jwt.Signed(signer).Claims(jwt.Claims{
		Issuer: fi.issuer, Subject: sub, Audience: jwt.Audience{oidcAudience},
		Expiry: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)), IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
	}).Claims(map[string]any{
		"repository_id": repoID, "repository_owner_id": ownerID, "ref": ref,
	}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func seedRepoGrant(t *testing.T, s *Server, slug, issuer string, repoID, ownerID int64, ref string) {
	t.Helper()
	seedGrant(t, s, slug, issuer, "placeholder-"+slug)
	if _, err := s.DB.Exec(`UPDATE oidc_grants SET subject = ?, repository_id = ?, repository_owner_id = ?, ref = ?
		WHERE subject = ?`, repositorySubject(repoID), repoID, ownerID, ref, "placeholder-"+slug); err != nil {
		t.Fatal(err)
	}
}

func TestOidcRepositoryGrant(t *testing.T) {
	s, ts := newTestServer(t)
	fi := newFakeIssuer(t)
	seedRepoGrant(t, s, "repoapp", fi.issuer, 1001, 501, "refs/heads/trunk")

	// Any subject format works: the grant matches on the repository claims.
	for _, sub := range []string{
		"repo:acme/app:ref:refs/heads/trunk",
		"repo:acme@501/app@1001:ref:refs/heads/trunk",
		"repo:acme/app:environment:production",
	} {
		code, tok := exchange(t, ts, fi.repoToken(t, sub, "1001", "501", "refs/heads/trunk"))
		if code != 200 || tok == "" {
			t.Fatalf("sub %q: exchange = %d", sub, code)
		}
	}
	cases := map[string]string{
		"other ref":   fi.repoToken(t, "repo:acme/app:pull_request", "1001", "501", "refs/pull/1/merge"),
		"other owner": fi.repoToken(t, "repo:evil/app:ref:refs/heads/trunk", "1001", "999", "refs/heads/trunk"),
		"other repo":  fi.repoToken(t, "repo:acme/other:ref:refs/heads/trunk", "1002", "501", "refs/heads/trunk"),
		// The synthetic subject never matches: it only keeps UNIQUE(issuer, subject).
		"synthetic sub": fi.repoToken(t, repositorySubject(1001), "", "", ""),
	}
	for name, tok := range cases {
		if code, _ := exchange(t, ts, tok); code != 401 {
			t.Errorf("%s: exchange = %d, want 401", name, code)
		}
	}
}

// A subject grant still works for a token that carries repository claims
// but has no repository grant.
func TestOidcSubjectGrantStillWorksWithRepoClaims(t *testing.T) {
	s, ts := newTestServer(t)
	fi := newFakeIssuer(t)
	seedGrant(t, s, "subjapp", fi.issuer, "repo:acme/app:ref:refs/heads/main")
	if code, _ := exchange(t, ts, fi.repoToken(t, "repo:acme/app:ref:refs/heads/main", "1001", "501", "refs/heads/main")); code != 200 {
		t.Fatalf("subject grant exchange = %d", code)
	}
}

// ---- GitHub App: manifest flow, install, connect ----

// fakeGithub is a minimal GitHub REST API: manifest conversion, app
// installations, installation tokens, one repository and its variables.
type fakeGithub struct {
	t   *testing.T
	srv *httptest.Server
	key *rsa.PrivateKey

	mu   sync.Mutex
	vars map[string]string
	// installPerms is what the installation was granted.
	installPerms map[string]string
}

func newFakeGithub(t *testing.T) *fakeGithub {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fg := &fakeGithub{t: t, key: key, vars: map[string]string{},
		installPerms: map[string]string{"metadata": "read", "actions_variables": "write"}}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	repo := map[string]any{"id": 1001, "full_name": "acme/app", "default_branch": "trunk", "private": true,
		"owner": map[string]any{"id": 501, "login": "acme"}}
	installation := func() map[string]any {
		fg.mu.Lock()
		defer fg.mu.Unlock()
		return map[string]any{"id": 7, "permissions": fg.installPerms, "account": map[string]any{"login": "acme"}}
	}
	reply := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if v != nil {
			_ = json.NewEncoder(w).Encode(v)
		}
	}
	appAuth := func(w http.ResponseWriter, r *http.Request) bool {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		tok, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.RS256})
		var c jwt.Claims
		if err != nil || tok.Claims(&key.PublicKey, &c) != nil || c.Issuer != "42" || c.ValidateWithLeeway(jwt.Expected{}, 0) != nil {
			reply(w, 401, map[string]string{"message": "Bad credentials"})
			return false
		}
		return true
	}
	instAuth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer ghs_test" {
			reply(w, 401, map[string]string{"message": "Bad credentials"})
			return false
		}
		return true
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /app-manifests/{code}/conversions", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("code") != "good-code" {
			reply(w, 404, map[string]string{"message": "Not Found"})
			return
		}
		reply(w, 201, map[string]any{"id": 42, "slug": "releasehub-abcd1234", "name": "ReleaseHub-abcd1234",
			"html_url": "https://github.com/apps/releasehub-abcd1234", "pem": pemText,
			"owner": map[string]any{"login": "acme"}})
	})
	mux.HandleFunc("GET /app/installations", func(w http.ResponseWriter, r *http.Request) {
		if appAuth(w, r) {
			reply(w, 200, []any{installation()})
		}
	})
	mux.HandleFunc("POST /app/installations/7/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		if appAuth(w, r) {
			reply(w, 201, map[string]any{"token": "ghs_test", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
		}
	})
	mux.HandleFunc("GET /installation/repositories", func(w http.ResponseWriter, r *http.Request) {
		if instAuth(w, r) {
			reply(w, 200, map[string]any{"total_count": 1, "repositories": []any{repo}})
		}
	})
	mux.HandleFunc("GET /repos/acme/app/installation", func(w http.ResponseWriter, r *http.Request) {
		if appAuth(w, r) {
			reply(w, 200, installation())
		}
	})
	mux.HandleFunc("GET /repos/acme/app", func(w http.ResponseWriter, r *http.Request) {
		if instAuth(w, r) {
			reply(w, 200, repo)
		}
	})
	mux.HandleFunc("GET /repos/acme/app/actions/variables/{name}", func(w http.ResponseWriter, r *http.Request) {
		if !instAuth(w, r) {
			return
		}
		fg.mu.Lock()
		v, ok := fg.vars[r.PathValue("name")]
		fg.mu.Unlock()
		if !ok {
			reply(w, 404, map[string]string{"message": "Not Found"})
			return
		}
		reply(w, 200, map[string]string{"name": r.PathValue("name"), "value": v})
	})
	setVar := func(w http.ResponseWriter, r *http.Request, status int) {
		if !instAuth(w, r) {
			return
		}
		var b struct{ Name, Value string }
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &b)
		fg.mu.Lock()
		_, exists := fg.vars[b.Name]
		if (status == 201) == exists { // create of an existing / update of a missing variable
			fg.mu.Unlock()
			reply(w, 409, map[string]string{"message": "conflict"})
			return
		}
		fg.vars[b.Name] = b.Value
		fg.mu.Unlock()
		reply(w, status, nil)
	}
	mux.HandleFunc("POST /repos/acme/app/actions/variables", func(w http.ResponseWriter, r *http.Request) { setVar(w, r, 201) })
	mux.HandleFunc("PATCH /repos/acme/app/actions/variables/{name}", func(w http.ResponseWriter, r *http.Request) { setVar(w, r, 204) })
	mux.HandleFunc("DELETE /repos/acme/app/actions/variables/{name}", func(w http.ResponseWriter, r *http.Request) {
		if !instAuth(w, r) {
			return
		}
		fg.mu.Lock()
		delete(fg.vars, r.PathValue("name"))
		fg.mu.Unlock()
		reply(w, 204, nil)
	})
	fg.srv = httptest.NewServer(mux)
	t.Cleanup(fg.srv.Close)
	return fg
}

func newGithubTestServer(t *testing.T) (*Server, *httptest.Server, *fakeGithub, string) {
	t.Helper()
	t.Setenv("RELEASE_HUB_SECRET_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	s, ts := newTestServer(t)
	fg := newFakeGithub(t)
	s.baseURL = "https://release-hub.test"
	s.githubAPI = fg.srv.URL
	s.githubWeb = "https://github.test"
	return s, ts, fg, seedSession(t, s)
}

// browserGet follows one GitHub redirect back to the release hub: session
// and flow cookies, no redirect following. Returns the Location.
func browserGet(t *testing.T, ts *httptest.Server, sess, flow, path string) string {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	if sess != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sess})
	}
	if flow != "" {
		req.AddCookie(&http.Cookie{Name: githubFlowCookie, Value: flow})
	}
	c := *ts.Client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET %s: status %d, want 303", path, resp.StatusCode)
	}
	return resp.Header.Get("Location")
}

// startFlow POSTs a flow start and returns the reply and the flow cookie.
func startFlow(t *testing.T, ts *httptest.Server, sess, path string, body string) (map[string]any, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sess})
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var env struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&env)
	if !env.Success {
		t.Fatalf("POST %s: status %d", path, resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == githubFlowCookie {
			if !c.HttpOnly || c.Path != githubFlowPath {
				t.Fatalf("flow cookie must be httpOnly and scoped to %s: %+v", githubFlowPath, c)
			}
			return env.Data, c.Value
		}
	}
	t.Fatalf("POST %s set no flow cookie", path)
	return nil, ""
}

func stateOf(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("state")
}

// registerGithubApp runs the manifest flow end to end.
func registerGithubApp(t *testing.T, ts *httptest.Server, sess string) {
	t.Helper()
	data, flow := startFlow(t, ts, sess, "/api/v1/github/manifest", `{}`)
	state := stateOf(t, data["url"].(string))
	loc := browserGet(t, ts, sess, flow, "/api/v1/github/callback?code=good-code&state="+state)
	if loc != "/settings/github?github=connected" {
		t.Fatalf("callback redirected to %s", loc)
	}
}

func TestGithubManifestFlow(t *testing.T) {
	s, ts, _, sess := newGithubTestServer(t)

	data, flow := startFlow(t, ts, sess, "/api/v1/github/manifest", `{"organization":"acme"}`)
	if u := data["url"].(string); !strings.HasPrefix(u, "https://github.test/organizations/acme/settings/apps/new?state=") {
		t.Fatalf("manifest url = %s", u)
	}
	m := data["manifest"].(map[string]any)
	if m["redirect_url"] != "https://release-hub.test/api/v1/github/callback" ||
		m["setup_url"] != "https://release-hub.test/api/v1/github/installed" ||
		m["public"] != false || !strings.HasPrefix(m["name"].(string), "ReleaseHub-") {
		t.Fatalf("manifest = %v", m)
	}
	if hook := m["hook_attributes"].(map[string]any); hook["active"] != false {
		t.Fatalf("webhooks must be off: %v", hook)
	}
	perms := m["default_permissions"].(map[string]any)
	if len(perms) != 2 || perms["actions_variables"] != "write" || perms["metadata"] != "read" {
		t.Fatalf("permissions = %v", perms)
	}
	state := stateOf(t, data["url"].(string))

	// Without the browser's flow cookie, or without a session, or with a
	// wrong state: refused.
	for _, c := range []struct{ sess, flow, state string }{
		{sess, "", state}, {"", flow, state}, {sess, flow, strings.Repeat("0", 48)}, {sess, "wrong-binding", state},
	} {
		if loc := browserGet(t, ts, c.sess, c.flow, "/api/v1/github/callback?code=good-code&state="+c.state); loc != "/settings/github?github=error" {
			t.Fatalf("callback %+v redirected to %s", c, loc)
		}
	}
	if loc := browserGet(t, ts, sess, flow, "/api/v1/github/callback?code=good-code&state="+state); loc != "/settings/github?github=connected" {
		t.Fatalf("callback redirected to %s", loc)
	}
	// A flow is single use.
	if loc := browserGet(t, ts, sess, flow, "/api/v1/github/callback?code=good-code&state="+state); loc != "/settings/github?github=error" {
		t.Fatalf("replayed callback redirected to %s", loc)
	}

	var st struct {
		App *githubAppInfo `json:"app"`
	}
	v1Do(t, ts, sess, "GET", "/api/v1/github", nil, 200, &st)
	if st.App == nil || st.App.Slug != "releasehub-abcd1234" || st.App.Owner != "acme" ||
		st.App.InstallURL != "https://github.test/apps/releasehub-abcd1234/installations/new" {
		t.Fatalf("settings app = %+v", st.App)
	}
	var stored string
	if err := s.DB.QueryRow("SELECT private_key FROM github_app").Scan(&stored); err != nil || !strings.HasPrefix(stored, "aes:") {
		t.Fatalf("private key must be stored sealed, got %.10q (%v)", stored, err)
	}
	// One app per release hub.
	v1Do(t, ts, sess, "POST", "/api/v1/github/manifest", map[string]any{}, 409, nil)

	// Disconnect forgets it locally.
	v1Do(t, ts, sess, "POST", "/api/v1/github/disconnect", map[string]any{}, 200, nil)
	v1Do(t, ts, sess, "GET", "/api/v1/github", nil, 200, &st)
	if st.App != nil {
		t.Fatal("app still connected after disconnect")
	}
}

func TestGithubManifestNeedsPublicHTTPSBaseURL(t *testing.T) {
	s, ts, _, sess := newGithubTestServer(t)
	for _, base := range []string{"http://localhost:9100", "http://release-hub.test", "https://localhost"} {
		s.baseURL = base
		v1Do(t, ts, sess, "POST", "/api/v1/github/manifest", map[string]any{}, 400, nil)
	}
}

func TestGithubInstallAndConnect(t *testing.T) {
	s, ts, fg, sess := newGithubTestServer(t)
	registerGithubApp(t, ts, sess)
	v1Do(t, ts, sess, "POST", "/api/v1/apps", map[string]any{"slug": "goldview"}, 201, nil)

	// Install round trip: the installation id must belong to the app.
	data, flow := startFlow(t, ts, sess, "/api/v1/github/install", `{}`)
	state := stateOf(t, data["url"].(string))
	if loc := browserGet(t, ts, sess, flow, "/api/v1/github/installed?installation_id=999&state="+state); loc != "/settings/github?github=error" {
		t.Fatalf("foreign installation redirected to %s", loc)
	}
	data, flow = startFlow(t, ts, sess, "/api/v1/github/install", `{}`)
	state = stateOf(t, data["url"].(string))
	if loc := browserGet(t, ts, sess, flow, "/api/v1/github/installed?installation_id=7&setup_action=install&state="+state); loc != "/settings/github?github=installed" {
		t.Fatalf("install redirected to %s", loc)
	}

	var repos []struct {
		FullName      string  `json:"fullName"`
		DefaultBranch string  `json:"defaultBranch"`
		App           *string `json:"app"`
	}
	v1Do(t, ts, sess, "GET", "/api/v1/github/repositories", nil, 200, &repos)
	if len(repos) != 1 || repos[0].FullName != "acme/app" || repos[0].App != nil {
		t.Fatalf("repositories = %+v", repos)
	}

	// An existing variable with another value is a conflict: nothing changes.
	fg.vars[varReleaseHubURL] = "https://old.example.com"
	var res struct {
		Connected bool                  `json:"connected"`
		Ref       string                `json:"ref"`
		Conflicts []githubVariableState `json:"conflicts"`
	}
	v1Do(t, ts, sess, "POST", "/api/v1/apps/goldview/github/connect", map[string]any{"repository": "acme/app"}, 200, &res)
	if res.Connected || len(res.Conflicts) != 1 || *res.Conflicts[0].Current != "https://old.example.com" {
		t.Fatalf("conflict reply = %+v", res)
	}
	if _, ok := fg.vars[varReleaseHubApp]; ok {
		t.Fatal("a conflict must not change any variable")
	}
	var n int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM oidc_grants").Scan(&n)
	if n != 0 {
		t.Fatal("a conflict must not create a grant")
	}

	v1Do(t, ts, sess, "POST", "/api/v1/apps/goldview/github/connect", map[string]any{"repository": "acme/app", "overwrite": true}, 200, &res)
	if !res.Connected || res.Ref != "refs/heads/trunk" {
		t.Fatalf("connect reply = %+v", res)
	}
	if fg.vars[varReleaseHubURL] != "https://release-hub.test" || fg.vars[varReleaseHubApp] != "goldview" {
		t.Fatalf("variables = %v", fg.vars)
	}
	var grants []struct {
		ID           int64  `json:"id"`
		Name         string `json:"name"`
		RepositoryID *int64 `json:"repositoryId"`
		Ref          string `json:"ref"`
	}
	v1Do(t, ts, sess, "GET", "/api/v1/apps/goldview/oidc/grants", nil, 200, &grants)
	if len(grants) != 1 || grants[0].Name != "acme/app" || grants[0].RepositoryID == nil || *grants[0].RepositoryID != 1001 || grants[0].Ref != "refs/heads/trunk" {
		t.Fatalf("grants = %+v", grants)
	}
	v1Do(t, ts, sess, "GET", "/api/v1/github/repositories", nil, 200, &repos)
	if repos[0].App == nil || *repos[0].App != "goldview" {
		t.Fatalf("repo should show its app: %+v", repos)
	}

	// One repo → one app.
	v1Do(t, ts, sess, "POST", "/api/v1/apps", map[string]any{"slug": "other"}, 201, nil)
	v1Do(t, ts, sess, "POST", "/api/v1/apps/other/github/connect", map[string]any{"repository": "acme/app"}, 409, nil)

	// Removing the grant with variables=1 also removes release-hub's variables.
	var del struct {
		VariablesRemoved bool `json:"variablesRemoved"`
	}
	v1Do(t, ts, sess, "DELETE", "/api/v1/oidc/grants/"+itoa(grants[0].ID)+"?variables=1", nil, 200, &del)
	if !del.VariablesRemoved || len(fg.vars) != 0 {
		t.Fatalf("variables after removal = %v (%+v)", fg.vars, del)
	}
}

func TestGithubConnectNeedsVariablesPermission(t *testing.T) {
	_, ts, fg, sess := newGithubTestServer(t)
	registerGithubApp(t, ts, sess)
	v1Do(t, ts, sess, "POST", "/api/v1/apps", map[string]any{"slug": "goldview"}, 201, nil)
	fg.installPerms = map[string]string{"metadata": "read"}
	env := v1Do(t, ts, sess, "POST", "/api/v1/apps/goldview/github/connect", map[string]any{"repository": "acme/app"}, 400, nil)
	if !strings.Contains(env.Error.Message, "Variables") {
		t.Fatalf("error = %s", env.Error.Message)
	}
	var repos []any
	v1Do(t, ts, sess, "GET", "/api/v1/github/repositories", nil, 200, &repos)
	if len(repos) != 0 {
		t.Fatalf("an installation without Variables permission must not list repos: %v", repos)
	}
}
