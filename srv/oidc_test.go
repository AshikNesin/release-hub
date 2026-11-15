package srv

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// fakeIssuer is a minimal OIDC issuer: discovery + JWKS + token minting.
type fakeIssuer struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	issuer string

	mu   sync.Mutex
	keys []jose.JSONWebKey // served by /jwks; grows via addKey
}

func newFakeIssuer(t *testing.T) *fakeIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fi := &fakeIssuer{key: key}
	fi.keys = []jose.JSONWebKey{{Key: key.Public(), KeyID: "test-key", Algorithm: string(jose.RS256), Use: "sig"}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":   fi.issuer, // set once the server URL is known
			"jwks_uri": fi.issuer + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		fi.mu.Lock()
		keys := append([]jose.JSONWebKey(nil), fi.keys...)
		fi.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: keys})
	})
	fi.srv = httptest.NewServer(mux)
	fi.issuer = fi.srv.URL
	t.Cleanup(fi.srv.Close)
	return fi
}

// addKey publishes an extra signing key, as an issuer rotation would.
func (fi *fakeIssuer) addKey(k interface{}, kid string) {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	fi.keys = append(fi.keys, jose.JSONWebKey{Key: k, KeyID: kid, Algorithm: string(jose.RS256), Use: "sig"})
}

// token mints a signed identity token from the fake issuer.
func (fi *fakeIssuer) token(t *testing.T, iss, sub, aud string, expires time.Time) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: fi.key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key"))
	if err != nil {
		t.Fatal(err)
	}
	claims := jwt.Claims{
		Issuer:   iss,
		Subject:  sub,
		Audience: jwt.Audience{aud},
		Expiry:   jwt.NewNumericDate(expires),
		IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
	}
	s, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// seedGrant inserts an app, an android platform, and an OIDC grant directly.
func seedGrant(t *testing.T, s *Server, slug, issuer, subject string) {
	t.Helper()
	if _, err := s.DB.Exec("INSERT INTO apps (slug) VALUES (?)", slug); err != nil {
		t.Fatal(err)
	}
	var appID int64
	if err := s.DB.QueryRow("SELECT id FROM apps WHERE slug = ?", slug).Scan(&appID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO app_platforms (app_id, platform, package_name) VALUES (?, 'android', ?)",
		appID, "io.test."+slug); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO oidc_grants (app_id, issuer, subject, name) VALUES (?, ?, ?, 'test')", appID, issuer, subject); err != nil {
		t.Fatal(err)
	}
}

// exchange posts an OIDC token to the exchange endpoint and decodes the reply.
func exchange(t *testing.T, ts *httptest.Server, oidcToken string) (int, string) {
	t.Helper()
	resp, err := ts.Client().Post(ts.URL+"/api/oidc/exchange", "application/x-www-form-urlencoded",
		strings.NewReader("token="+oidcToken))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expiresIn"`
		App       string `json:"app"`
		Error     string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Token
}

// oidcUpload uploads a tiny fake artifact with the given bearer token.
func oidcUpload(t *testing.T, ts *httptest.Server, bearer, slug string) int {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "app.apk")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte("apk-bytes"))
	_ = w.WriteField("channel", "direct")
	_ = w.WriteField("versionCode", "101")
	_ = w.Close()
	req, _ := http.NewRequest("POST", ts.URL+"/api/apps/"+slug+"/releases", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestOidcExchangeMintsScopedToken(t *testing.T) {
	fi := newFakeIssuer(t)
	sub := "repo:acme/app-one:ref:refs/heads/main"
	s, ts := newTestServer(t)
	seedGrant(t, s, "app-one", fi.issuer, sub)
	seedGrant(t, s, "app-two", fi.issuer, "repo:acme/app-two:ref:refs/heads/main")

	code, hubTok := exchange(t, ts, fi.token(t, fi.issuer, sub, oidcAudience, time.Now().Add(10*time.Minute)))
	if code != 200 || !strings.HasPrefix(hubTok, "rh_") {
		t.Fatalf("exchange: code=%d token=%q", code, hubTok)
	}

	// Publish to the granted app: allowed.
	if c := oidcUpload(t, ts, hubTok, "app-one"); c != 201 {
		t.Fatalf("scoped upload to its own app: %d", c)
	}
	// Publish to another app: refused, even though the same token hash is valid.
	if c := oidcUpload(t, ts, hubTok, "app-two"); c != 403 {
		t.Fatalf("scoped upload to another app: %d, want 403", c)
	}
	// Hub-wide endpoints: refused.
	req, _ := http.NewRequest("GET", ts.URL+"/api/apps", nil)
	req.Header.Set("Authorization", "Bearer "+hubTok)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("scoped token on /api/apps: %d, want 403", resp.StatusCode)
	}
	// Signing-key fetch on the granted app: allowed (CI signs with it).
	req, _ = http.NewRequest("GET", ts.URL+"/api/apps/app-one/android/signing", nil)
	req.Header.Set("Authorization", "Bearer "+hubTok)
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == 403 {
		t.Fatal("scoped token must reach the signing endpoint of its own app")
	}
	// Minted token shows up in the tokens list with its scope name.
	var toks []struct {
		Name string `json:"name"`
	}
	sess := seedSession(t, s)
	v1Do(t, ts, sess, "GET", "/api/v1/tokens", nil, 200, &toks)
	found := false
	for _, tk := range toks {
		if strings.Contains(tk.Name, "app-one") {
			found = true
		}
	}
	if !found {
		t.Fatalf("minted token missing from inventory: %+v", toks)
	}
}

func TestOidcExchangeRejectsBadTokens(t *testing.T) {
	fi := newFakeIssuer(t)
	s, ts := newTestServer(t)
	sub := "repo:acme/app-one:ref:refs/heads/main"
	seedGrant(t, s, "app-one", fi.issuer, sub)

	cases := map[string]func() (int, string){
		"expired": func() (int, string) {
			return exchange(t, ts, fi.token(t, fi.issuer, sub, oidcAudience, time.Now().Add(-time.Hour)))
		},
		"wrong audience": func() (int, string) {
			return exchange(t, ts, fi.token(t, fi.issuer, sub, "other-audience", time.Now().Add(10*time.Minute)))
		},
		"unregistered subject": func() (int, string) {
			return exchange(t, ts, fi.token(t, fi.issuer, "repo:acme/other:ref:refs/heads/main", oidcAudience, time.Now().Add(10*time.Minute)))
		},
		"garbage": func() (int, string) {
			return exchange(t, ts, "not-a-jwt")
		},
		"empty": func() (int, string) {
			return exchange(t, ts, "")
		},
		"unregistered issuer": func() (int, string) {
			return exchange(t, ts, fi.token(t, "https://evil.example", sub, oidcAudience, time.Now().Add(10*time.Minute)))
		},
	}
	for name, run := range cases {
		code, tok := run()
		if code != 401 || tok != "" {
			t.Fatalf("%s: code=%d token=%q, want 401/no token", name, code, tok)
		}
	}
}

func TestOidcGrantRevocationKillsTokens(t *testing.T) {
	fi := newFakeIssuer(t)
	sub := "repo:acme/app-one:ref:refs/heads/main"
	s, ts := newTestServer(t)
	seedGrant(t, s, "app-one", fi.issuer, sub)

	_, hubTok := exchange(t, ts, fi.token(t, fi.issuer, sub, oidcAudience, time.Now().Add(10*time.Minute)))
	if c := oidcUpload(t, ts, hubTok, "app-one"); c != 201 {
		t.Fatalf("upload before revoke: %d", c)
	}

	// Revoke via the UI API: outstanding minted tokens must stop working.
	sess := seedSession(t, s)
	var grants []struct {
		ID int64 `json:"id"`
	}
	v1Do(t, ts, sess, "GET", "/api/v1/apps/app-one/oidc/grants", nil, 200, &grants)
	if len(grants) != 1 {
		t.Fatalf("grants: %+v", grants)
	}
	v1Do(t, ts, sess, "DELETE", "/api/v1/oidc/grants/"+itoa(grants[0].ID), nil, 200, nil)
	if c := oidcUpload(t, ts, hubTok, "app-one"); c != 401 {
		t.Fatalf("upload after grant revoke: %d, want 401", c)
	}
}

func TestOidcGrantOneRepoOneApp(t *testing.T) {
	fi := newFakeIssuer(t)
	s, ts := newTestServer(t)
	sess := seedSession(t, s)
	sub := "repo:acme/app-one:ref:refs/heads/main"

	// Register the app first, then grant.
	v1Do(t, ts, sess, "POST", "/api/v1/apps", map[string]string{"slug": "app-one"}, 201, nil)
	v1Do(t, ts, sess, "POST", "/api/v1/apps", map[string]string{"slug": "app-two"}, 201, nil)
	v1Do(t, ts, sess, "POST", "/api/v1/apps/app-one/oidc/grants", map[string]string{
		"issuer": fi.issuer, "subject": sub, "name": "acme/app-one CI",
	}, 200, nil)

	// The same identity cannot be granted to a second app.
	env := v1Do(t, ts, sess, "POST", "/api/v1/apps/app-two/oidc/grants", map[string]string{
		"issuer": fi.issuer, "subject": sub,
	}, 409, nil)
	if !strings.Contains(env.Error.Message, "one repo") {
		t.Fatalf("conflict message: %+v", env.Error)
	}
}

func TestOidcMintedTokenExpires(t *testing.T) {
	fi := newFakeIssuer(t)
	sub := "repo:acme/app-one:ref:refs/heads/main"
	s, ts := newTestServer(t)
	seedGrant(t, s, "app-one", fi.issuer, sub)

	// Mint a token that expired a minute ago by backdating the row.
	_, hubTok := exchange(t, ts, fi.token(t, fi.issuer, sub, oidcAudience, time.Now().Add(10*time.Minute)))
	_, err := s.DB.Exec("UPDATE api_tokens SET expires_at = ? WHERE token_hash = ?",
		time.Now().Add(-time.Minute), hashToken(hubTok))
	if err != nil {
		t.Fatal(err)
	}
	if c := oidcUpload(t, ts, hubTok, "app-one"); c != 401 {
		t.Fatalf("expired minted token: %d, want 401", c)
	}
	// …and the row is gone.
	var n int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM api_tokens WHERE token_hash = ?", hashToken(hubTok)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("expired minted token must be deleted on sight")
	}
}

func TestOidcJwksRotate(t *testing.T) {
	fi := newFakeIssuer(t)
	sub := "repo:acme/app-one:ref:refs/heads/main"
	s, ts := newTestServer(t)
	seedGrant(t, s, "app-one", fi.issuer, sub)

	// Token signed by a second key the issuer later publishes.
	key2, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer2, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key2}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "key-2"))
	if err != nil {
		t.Fatal(err)
	}
	signed2, err := jwt.Signed(signer2).Claims(jwt.Claims{
		Issuer: fi.issuer, Subject: sub, Audience: jwt.Audience{oidcAudience},
		Expiry: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
	}).Serialize()
	if err != nil {
		t.Fatal(err)
	}

	// Rejected before the key is published …
	if code, _ := exchange(t, ts, signed2); code != 401 {
		t.Fatalf("unknown kid accepted: %d", code)
	}
	// … accepted once the issuer adds it (cache must not pin the old set forever).
	fi.addKey(key2.Public(), "key-2")
	// The cache holds the old set for its TTL; a fresh server bypasses it.
	s2, ts2 := newTestServer(t)
	seedGrant(t, s2, "app-one", fi.issuer, sub)
	if code, tok := exchange(t, ts2, signed2); code != 200 || tok == "" {
		t.Fatalf("rotated key rejected: code=%d", code)
	}
}

func TestOidcScopePathMatching(t *testing.T) {
	allow := func(method, path string) bool {
		req, _ := http.NewRequest(method, "http://hub"+path, nil)
		return oidcScopeAllows(req, "app-one")
	}
	for _, c := range [][2]string{
		{"POST", "/api/apps/app-one/releases"},
		{"GET", "/api/apps/app-one/releases"},
		{"DELETE", "/api/apps/app-one/releases/42"},
		{"GET", "/api/apps/app-one/android/releases"},
		{"DELETE", "/api/apps/app-one/android/releases/42"},
		{"GET", "/api/apps/app-one/manifest"},
		{"GET", "/api/apps/app-one/android/manifest"},
		{"GET", "/api/apps/app-one/signing"},
		{"GET", "/api/apps/app-one/android/signing"},
	} {
		if !allow(c[0], c[1]) {
			t.Fatalf("%s %s must be allowed", c[0], c[1])
		}
	}
	for _, p := range []string{
		"/api/apps/app-two/releases",
		"/api/apps/app-one/play",
		"/api/apps/app-one/testers",
		"/api/apps/app-one/tracks",
		"/api/apps/app-one/platforms",
		"/api/apps/app-one",
		"/api/apps/app-one-extra/releases", // prefix must not match
		"/api/tokens",
		"/api/play-accounts",
	} {
		if allow("POST", p) {
			t.Fatalf("%s must be refused", p)
		}
	}
	if allow("POST", "/api/apps/app-one/android/signing") {
		t.Fatal("signing write must be refused")
	}
}

// ---- helpers ----
