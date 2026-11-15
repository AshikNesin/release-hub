package srv

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// seedSession inserts a valid UI session and returns its cookie value.
func seedSession(t *testing.T, s *Server) string {
	t.Helper()
	sess := "sess_" + strings.ReplaceAll(t.Name(), "/", "_")
	if _, err := s.DB.Exec(
		"INSERT INTO sessions (token_hash, created_at, expires_at) VALUES (?, datetime('now'), datetime('now', '+1 day'))",
		hashToken(sess)); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return sess
}

type v1Envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// v1Do sends a JSON request with the session cookie (sess may be ""),
// asserts the status and envelope shape, and decodes data into out.
func v1Do(t *testing.T, ts *httptest.Server, sess, method, path string, body any, wantStatus int, out any) v1Envelope {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, ts.URL+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if sess != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sess})
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, resp.StatusCode, wantStatus, raw)
	}
	var env v1Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("%s %s: not an envelope: %s", method, path, raw)
	}
	if env.Success != (wantStatus < 400) || (wantStatus >= 400 && env.Error == nil) {
		t.Fatalf("%s %s: envelope mismatch: %s", method, path, raw)
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			t.Fatalf("%s %s: decode data: %v (%s)", method, path, err, raw)
		}
	}
	return env
}

func TestV1AuthFlow(t *testing.T) {
	_, ts := newTestServer(t)

	var st struct{ NeedsSetup bool }
	v1Do(t, ts, "", "GET", "/api/v1/auth/setup", nil, 200, &st)
	if !st.NeedsSetup {
		t.Fatal("fresh release hub must need setup")
	}
	env := v1Do(t, ts, "", "GET", "/api/v1/auth/me", nil, 401, nil)
	if env.Error.Code != "SESSION_INVALID" {
		t.Fatalf("me without session: code %q, want SESSION_INVALID", env.Error.Code)
	}
	v1Do(t, ts, "", "POST", "/api/v1/auth/setup", map[string]string{"password": "short"}, 400, nil)

	// Setup sets the password and returns a session cookie.
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/setup", strings.NewReader(`{"password":"correct horse"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("setup: %d", resp.StatusCode)
	}
	var sess string
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			sess = c.Value
		}
	}
	if sess == "" {
		t.Fatal("setup must set the session cookie")
	}
	var me sessionUser
	v1Do(t, ts, sess, "GET", "/api/v1/auth/me", nil, 200, &me)
	if me.Name != "Admin" {
		t.Fatalf("me = %+v", me)
	}

	// Setup is first-run only; login checks the password.
	v1Do(t, ts, "", "POST", "/api/v1/auth/setup", map[string]string{"password": "another password"}, 403, nil)
	env = v1Do(t, ts, "", "POST", "/api/v1/auth/login", map[string]string{"password": "wrong password"}, 401, nil)
	if env.Error.Code == "SESSION_INVALID" {
		t.Fatal("a wrong password must not read as an expired session")
	}
	v1Do(t, ts, "", "POST", "/api/v1/auth/login", map[string]string{"password": "correct horse"}, 200, nil)

	// Logout ends the session server-side.
	v1Do(t, ts, sess, "POST", "/api/v1/auth/logout", nil, 200, nil)
	v1Do(t, ts, sess, "GET", "/api/v1/auth/me", nil, 401, nil)
}

func TestV1RejectsCrossSiteWrites(t *testing.T) {
	s, ts := newTestServer(t)
	sess := seedSession(t, s)
	for name, hdr := range map[string][2]string{
		"fetch metadata": {"Sec-Fetch-Site", "cross-site"},
		"origin":         {"Origin", "https://evil.example"},
	} {
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/apps", strings.NewReader(`{"slug":"evil"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(hdr[0], hdr[1])
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sess})
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatalf("%s: cross-site POST got %d, want 403", name, resp.StatusCode)
		}
	}
	var apps []struct{ Slug string }
	v1Do(t, ts, sess, "GET", "/api/v1/apps", nil, 200, &apps)
	if len(apps) != 0 {
		t.Fatalf("cross-site request created an app: %+v", apps)
	}
}

func TestV1AppsAndPlatforms(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	sess := seedSession(t, s)

	v1Do(t, ts, sess, "PATCH", "/api/v1/settings", map[string]string{"bundle_prefix": "io.nesin."}, 200, nil)
	v1Do(t, ts, sess, "POST", "/api/v1/apps", map[string]string{"slug": "Bad Slug"}, 400, nil)
	v1Do(t, ts, sess, "POST", "/api/v1/apps", map[string]string{"slug": "my-app"}, 201, nil)
	env := v1Do(t, ts, sess, "POST", "/api/v1/apps", map[string]string{"slug": "my-app"}, 409, nil)
	if env.Error.Code != "CONFLICT" {
		t.Fatalf("duplicate app: code %q", env.Error.Code)
	}

	var d struct {
		SuggestedPackage string
		Platforms        []struct{ Platform string }
	}
	v1Do(t, ts, sess, "GET", "/api/v1/apps/my-app", nil, 200, &d)
	if d.SuggestedPackage != "io.nesin.my_app" || len(d.Platforms) != 0 {
		t.Fatalf("detail before platforms: %+v", d)
	}
	var added map[string]string
	v1Do(t, ts, sess, "POST", "/api/v1/apps/my-app/platforms",
		map[string]string{"platform": "android", "packageName": "io.nesin.my_app"}, 201, &added)
	if added["signingKey"] != "generated" {
		t.Fatalf("android platform must get a key: %+v", added)
	}
	v1Do(t, ts, sess, "GET", "/api/v1/apps/missing", nil, 404, nil)

	// Retention override: set, then inherit again.
	var ret struct {
		Override  *int64
		Effective int64
	}
	v1Do(t, ts, sess, "PUT", "/api/v1/apps/my-app/android/retention", map[string]any{"keep": 3}, 200, &ret)
	if ret.Override == nil || *ret.Override != 3 || ret.Effective != 3 {
		t.Fatalf("retention after set: %+v", ret)
	}
	v1Do(t, ts, sess, "PUT", "/api/v1/apps/my-app/android/retention", map[string]any{"keep": -1}, 400, nil)
	v1Do(t, ts, sess, "PATCH", "/api/v1/settings", map[string]string{"prune_keep": "5"}, 200, nil)
	v1Do(t, ts, sess, "PUT", "/api/v1/apps/my-app/android/retention", map[string]any{"keep": nil}, 200, &ret)
	if ret.Override != nil || ret.Effective != 5 {
		t.Fatalf("retention after inherit: %+v", ret)
	}

	var list []struct {
		Slug      string
		Platforms []struct {
			Platform      string
			HasSigningKey bool
			Latest        *struct{}
		}
	}
	v1Do(t, ts, sess, "GET", "/api/v1/apps", nil, 200, &list)
	if len(list) != 1 || len(list[0].Platforms) != 1 || !list[0].Platforms[0].HasSigningKey || list[0].Platforms[0].Latest != nil {
		t.Fatalf("app list: %+v", list)
	}
}

func TestV1SettingsAndTokens(t *testing.T) {
	s, ts := newTestServer(t)
	sess := seedSession(t, s)

	type settings struct {
		BundlePrefix string
		PlayTesters  string
		PruneKeep    string
		CertFields   []struct{ Field, Value string }
	}
	var st settings
	v1Do(t, ts, sess, "PATCH", "/api/v1/settings", map[string]string{
		"play_testers": "B@googlegroups.com\na@googlegroups.com, b@googlegroups.com",
		"org":          "Nesin",
	}, 200, &st)
	if st.PlayTesters != "b@googlegroups.com, a@googlegroups.com" {
		t.Fatalf("testers not normalized: %q", st.PlayTesters)
	}
	// A partial save leaves other keys alone; a bad value saves nothing.
	v1Do(t, ts, sess, "PATCH", "/api/v1/settings", map[string]string{"prune_keep": "x", "org": "Other"}, 400, nil)
	v1Do(t, ts, sess, "GET", "/api/v1/settings", nil, 200, &st)
	for _, f := range st.CertFields {
		if f.Field == "org" && f.Value != "Nesin" {
			t.Fatalf("org = %q after a rejected save", f.Value)
		}
	}
	if st.PlayTesters == "" {
		t.Fatal("unrelated key cleared by a partial save")
	}

	// Tokens: create (secret shown once), list (no secret), use, revoke.
	var created struct{ Name, Token string }
	v1Do(t, ts, sess, "POST", "/api/v1/tokens", map[string]string{"name": "ci"}, 201, &created)
	if !strings.HasPrefix(created.Token, "rh_") {
		t.Fatalf("token = %q", created.Token)
	}
	var toks []struct {
		ID   int64
		Name string
	}
	v1Do(t, ts, sess, "GET", "/api/v1/tokens", nil, 200, &toks)
	if len(toks) != 1 || toks[0].Name != "ci" {
		t.Fatalf("tokens: %+v", toks)
	}
	bearer := func() int {
		req, _ := http.NewRequest("GET", ts.URL+"/api/apps", nil)
		req.Header.Set("Authorization", "Bearer "+created.Token)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := bearer(); code != 200 {
		t.Fatalf("new token rejected: %d", code)
	}
	v1Do(t, ts, sess, "DELETE", "/api/v1/tokens/"+itoa(toks[0].ID), nil, 200, nil)
	if code := bearer(); code != 401 {
		t.Fatalf("revoked token still works: %d", code)
	}
	// A bearer token never unlocks the session API.
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer "+created.Token)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("bearer on v1: %d, want 401", resp.StatusCode)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestSPAFallback(t *testing.T) {
	dir := t.TempDir()
	ui := fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html><div id=root></div>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
		"favicon.svg":     {Data: []byte("<svg/>")},
	}
	s, err := New(Options{
		DBPath: filepath.Join(dir, "db.sqlite3"), BaseURL: "http://release-hub.test",
		Storage: &LocalStorage{Dir: filepath.Join(dir, "artifacts"), BaseURL: "http://release-hub.test"},
		UI:      ui,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.routes())
	defer ts.Close()

	get := func(path string) (int, string, http.Header) {
		resp, err := ts.Client().Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), resp.Header
	}
	for _, p := range []string{"/", "/apps", "/apps/demo?tab=distribution", "/settings/tokens", "/login"} {
		if code, body, _ := get(p); code != 200 || !strings.Contains(body, "id=root") {
			t.Fatalf("%s: %d %q; want the SPA shell", p, code, body)
		}
	}
	if code, body, h := get("/assets/app-1.js"); code != 200 || body != "console.log(1)" || !strings.Contains(h.Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d %q %q", code, body, h.Get("Cache-Control"))
	}
	if code, _, _ := get("/assets/gone-0.js"); code != 404 {
		t.Fatalf("missing asset: %d, want 404 (not the HTML shell)", code)
	}
	if code, body, _ := get("/api/v1/nope"); code != 404 || !strings.Contains(body, `"success":false`) {
		t.Fatalf("unknown v1 route: %d %s", code, body)
	}
	if code, body, _ := get("/health"); code != 200 || body != "ok" {
		t.Fatalf("health: %d %q", code, body)
	}
}
