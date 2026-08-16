package srv

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	s, err := New(Options{
		DBPath: filepath.Join(dir, "test.sqlite3"), Hostname: "test-host",
		BaseURL: "http://hub.test",
		Storage: &LocalStorage{Dir: filepath.Join(dir, "artifacts"), BaseURL: "http://hub.test"},
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	return s, ts
}

func TestApiRequiresBearer(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/apps")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 without token, got %d", resp.StatusCode)
	}
}

func TestUploadAndManifestFlow(t *testing.T) {
	s, ts := newTestServer(t)
	client := ts.Client()

	// Seed a bearer token directly in the DB with a known hash.
	tok := "rh_testtoken123"
	if _, err := s.DB.Exec(
		"INSERT INTO api_tokens (name, token_hash) VALUES ('test', ?)", hashToken(tok)); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	// Create app (slug only), then add the android platform
	req, _ := http.NewRequest("POST", ts.URL+"/api/apps", bytes.NewBufferString("slug=demo"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create app: %d", resp.StatusCode)
	}
	req, _ = http.NewRequest("POST", ts.URL+"/api/apps/demo/platforms", bytes.NewBufferString("platform=android&packageName=io.demo"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("add platform: %d", resp.StatusCode)
	}

	// Upload a fake artifact
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("file", "demo.apk")
	fw.Write([]byte("fake-apk-bytes"))
	mw.WriteField("channel", "direct")
	mw.WriteField("versionCode", "10")
	mw.WriteField("versionName", "1.0")
	mw.Close()
	req, _ = http.NewRequest("POST", ts.URL+"/api/apps/demo/releases", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("upload: %d", resp.StatusCode)
	}

	// Manifest should be served publicly (no auth header)
	resp2, err := http.Get(ts.URL + "/api/apps/demo/manifest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("manifest: %d", resp2.StatusCode)
	}
	var m releaseManifest
	if err := json.NewDecoder(resp2.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m.VersionCode != 10 || m.VersionName != "1.0" {
		t.Fatalf("manifest content: %+v", m)
	}
	sum := sha256.Sum256([]byte("fake-apk-bytes"))
	if m.Sha256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha mismatch in manifest: %s", m.Sha256)
	}

	// unknown channel on manifest: 404, no panic
	respM, err := http.Get(ts.URL + "/api/apps/demo/manifest?channel=nosuch")
	if err != nil {
		t.Fatal(err)
	}
	respM.Body.Close()
	if respM.StatusCode != 404 {
		t.Fatalf("expected 404 for unknown channel, got %d", respM.StatusCode)
	}

	// share link: /apps/demo/download redirects to the latest direct
	// artifact; Play channels redirect to their Play URLs; ?channel works;
	// unknown channels 400 and empty channels 404 instead of panicking.
	// (/get is the legacy alias for the same handler.)
	client2 := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, path := range []string{"/apps/demo/download", "/apps/demo/get"} {
		respG, err := client2.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		respG.Body.Close()
		if respG.StatusCode != 302 {
			t.Fatalf("expected 302 from %s, got %d", path, respG.StatusCode)
		}
		if loc := respG.Header.Get("Location"); !strings.Contains(loc, "/artifacts/demo/android/10_demo.apk") {
			t.Fatalf("unexpected redirect target from %s: %s", path, loc)
		}
	}
	respP, err := client2.Get(ts.URL + "/apps/demo/download?channel=internal")
	if err != nil {
		t.Fatal(err)
	}
	respP.Body.Close()
	if respP.StatusCode != 302 || respP.Header.Get("Location") != "https://play.google.com/apps/testing/io.demo" {
		t.Fatalf("internal channel: %d %s", respP.StatusCode, respP.Header.Get("Location"))
	}
	respS, err := client2.Get(ts.URL + "/apps/demo/download?channel=public")
	if err != nil {
		t.Fatal(err)
	}
	respS.Body.Close()
	if respS.StatusCode != 302 || !strings.Contains(respS.Header.Get("Location"), "store/apps/details?id=io.demo") {
		t.Fatalf("public channel: %d %s", respS.StatusCode, respS.Header.Get("Location"))
	}
	respB, err := client2.Get(ts.URL + "/apps/demo/download?channel=bogus")
	if err != nil {
		t.Fatal(err)
	}
	respB.Body.Close()
	if respB.StatusCode != 400 {
		t.Fatalf("expected 400 for bogus channel, got %d", respB.StatusCode)
	}
	respG3, err := client2.Get(ts.URL + "/apps/demo/download?channel=direct")
	if err != nil {
		t.Fatal(err)
	}
	respG3.Body.Close()
	if respG3.StatusCode != 302 {
		t.Fatalf("expected 302 from /download?channel=direct, got %d", respG3.StatusCode)
	}
	respI, err := client2.Get(ts.URL + "/apps/demo/get/ios")
	if err != nil {
		t.Fatal(err)
	}
	respI.Body.Close()
	if respI.StatusCode != 404 {
		t.Fatalf("expected 404 for unknown platform on /get, got %d", respI.StatusCode)
	}

	// versionCode regression must be rejected
	body2 := &bytes.Buffer{}
	mw2 := multipart.NewWriter(body2)
	fw2, _ := mw2.CreateFormFile("file", "demo2.apk")
	fw2.Write([]byte("other"))
	mw2.WriteField("versionCode", "10")
	mw2.Close()
	req, _ = http.NewRequest("POST", ts.URL+"/api/apps/demo/releases", body2)
	req.Header.Set("Content-Type", mw2.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	resp3, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != 409 {
		t.Fatalf("expected 409 for version regression, got %d", resp3.StatusCode)
	}
}

// App page share row: direct always (stable /get link); internal/public
// (Play links) only while Play publishing is enabled for the platform.
func TestAppPageShareLinks(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	client := ts.Client()

	tok := "rh_sharetest"
	if _, err := s.DB.Exec(
		"INSERT INTO api_tokens (name, token_hash) VALUES ('test', ?)", hashToken(tok)); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	for _, f := range []struct{ path, body, ct string }{
		{"/api/apps", "slug=demo", "application/x-www-form-urlencoded"},
		{"/api/apps/demo/platforms", "platform=android&packageName=io.demo", "application/x-www-form-urlencoded"},
	} {
		req, _ := http.NewRequest("POST", ts.URL+f.path, bytes.NewBufferString(f.body))
		req.Header.Set("Content-Type", f.ct)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 201 {
			t.Fatalf("%s: %d", f.path, resp.StatusCode)
		}
	}

	// UI session cookie (page needs session auth).
	sess := "sess_sharetest"
	if _, err := s.DB.Exec(
		"INSERT INTO sessions (token_hash, created_at, expires_at) VALUES (?, datetime('now'), datetime('now', '+1 day'))",
		hashToken(sess)); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	get := func() string {
		req, _ := http.NewRequest("GET", ts.URL+"/apps/demo", nil)
		req.AddCookie(&http.Cookie{Name: "rh_session", Value: sess})
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("app page: %d", resp.StatusCode)
		}
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}

	// Play disabled: only the direct share link.
	page := get()
	if !strings.Contains(page, "data-copy=\"http://hub.test/apps/demo/download?channel=direct\"") {
		t.Fatal("missing direct share link")
	}
	if strings.Contains(page, "download?channel=\"") { // any share-row copy button beyond direct
		t.Fatal("Play share links must be hidden while Play publishing is disabled")
	}
	if strings.Contains(page, "invite testers") {
		t.Fatal("invite-testers button must be hidden while Play publishing is disabled")
	}

	// Enable Play → internal + public links appear, derived from the package.
	creds := []byte(`{"client_email": "sa@test.iam.gserviceaccount.com", "private_key": "x"}`)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("file", "sa.json")
	fw.Write(creds)
	mw.Close()
	req, _ := http.NewRequest("POST", ts.URL+"/api/apps/demo/play", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("set play: %d", resp.StatusCode)
	}

	page = get()
	if !strings.Contains(page, "data-copy=\"http://hub.test/apps/demo/download?channel=internal\"") {
		t.Fatal("missing internal share link")
	}
	if !strings.Contains(page, "data-copy=\"http://hub.test/apps/demo/download?channel=public\"") {
		t.Fatal("missing public share link")
	}
	if !strings.Contains(page, "invite testers") {
		t.Fatal("missing invite-testers button with Play enabled")
	}
}

// App → platforms: one slug, android + ios variants, independent releases
// and signing keys per platform.
func TestApiSetPlayUpdatesPlatformRowNotAppRow(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	client := ts.Client()

	tok := "rh_playtest"
	if _, err := s.DB.Exec(
		"INSERT INTO api_tokens (name, token_hash) VALUES ('test', ?)", hashToken(tok)); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	// Two apps, two android platforms — app ids and platform-row ids must NOT
	// line up for this regression test to bite.
	for _, slug := range []string{"alpha", "beta"} {
		req, _ := http.NewRequest("POST", ts.URL+"/api/apps", bytes.NewBufferString("slug="+slug))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		req, _ = http.NewRequest("POST", ts.URL+"/api/apps/"+slug+"/platforms",
			bytes.NewBufferString("platform=android&packageName=io.test."+slug))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err = client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	// Enable Play for the FIRST app's platform via the API.
	creds := []byte(`{"client_email": "sa@test.iam.gserviceaccount.com", "private_key": "x"}`)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("file", "sa.json")
	fw.Write(creds)
	mw.Close()
	req, _ := http.NewRequest("POST", ts.URL+"/api/apps/alpha/play", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("set play: %d", resp.StatusCode)
	}
	var out struct {
		PlayEnabled    bool   `json:"playEnabled"`
		ServiceAccount string `json:"serviceAccount"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.PlayEnabled || out.ServiceAccount != "sa@test.iam.gserviceaccount.com" {
		t.Fatalf("unexpected response: %+v", out)
	}

	// Exactly the alpha platform row must carry credentials — not beta's,
	// not some accidental id overlap.
	var alphaPlay, betaPlay int
	s.DB.QueryRow(`SELECT ap.play_enabled FROM app_platforms ap
		JOIN apps a ON a.id = ap.app_id WHERE a.slug = 'alpha'`).Scan(&alphaPlay)
	s.DB.QueryRow(`SELECT ap.play_enabled FROM app_platforms ap
		JOIN apps a ON a.id = ap.app_id WHERE a.slug = 'beta'`).Scan(&betaPlay)
	if alphaPlay != 1 || betaPlay != 0 {
		t.Fatalf("play flags wrong: alpha=%d beta=%d (app/platform id mixup)", alphaPlay, betaPlay)
	}
}

func TestSharedPlayAccountFlow(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	client := ts.Client()

	tok := "rh_sharedplay"
	if _, err := s.DB.Exec(
		"INSERT INTO api_tokens (name, token_hash) VALUES ('test', ?)", hashToken(tok)); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	// Three apps → three platforms sharing one service account.
	for _, slug := range []string{"one", "two", "three"} {
		req, _ := http.NewRequest("POST", ts.URL+"/api/apps", bytes.NewBufferString("slug="+slug))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		req, _ = http.NewRequest("POST", ts.URL+"/api/apps/"+slug+"/platforms",
			bytes.NewBufferString("platform=android&packageName=io.test."+slug))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err = client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	// Create the shared account once.
	creds := []byte(`{"client_email": "shared@test.iam.gserviceaccount.com"}`)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("file", "sa.json")
	fw.Write(creds)
	mw.Close()
	req, _ := http.NewRequest("POST", ts.URL+"/api/play-accounts", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var acct struct {
		ID             int64  `json:"id"`
		ServiceAccount string `json:"serviceAccount"`
	}
	json.NewDecoder(resp.Body).Decode(&acct)
	resp.Body.Close()
	if acct.ID == 0 || acct.ServiceAccount != "shared@test.iam.gserviceaccount.com" {
		t.Fatalf("create account: %+v", acct)
	}

	// Enable every app against it with one form field — no file re-upload.
	for _, slug := range []string{"one", "two", "three"} {
		req, _ := http.NewRequest("POST", ts.URL+"/api/apps/"+slug+"/play",
			bytes.NewBufferString("account=1"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		out, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(out), "playEnabled\":true") {
			t.Fatalf("enable %s: %d %s", slug, resp.StatusCode, out)
		}
	}

	// One credential row, three enabled platforms pointing at it.
	var nAccts, nEnabled int
	s.DB.QueryRow("SELECT COUNT(*) FROM play_accounts").Scan(&nAccts)
	s.DB.QueryRow("SELECT COUNT(*) FROM app_platforms WHERE play_enabled = 1 AND play_account_id = 1").Scan(&nEnabled)
	if nAccts != 1 || nEnabled != 3 {
		t.Fatalf("want 1 account / 3 enabled platforms, got %d / %d", nAccts, nEnabled)
	}

	// Delete the account: platforms detach and disable.
	req, _ = http.NewRequest("POST", ts.URL+"/api/play-accounts/delete", bytes.NewBufferString("id=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	s.DB.QueryRow("SELECT COUNT(*) FROM app_platforms WHERE play_enabled = 1").Scan(&nEnabled)
	if nEnabled != 0 {
		t.Fatalf("platforms still enabled after account delete: %d", nEnabled)
	}
}

func TestAppPlatforms(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	tok := "rh_plat_test"
	if _, err := s.DB.Exec("INSERT INTO api_tokens (name, token_hash) VALUES ('t', ?)", hashToken(tok)); err != nil {
		t.Fatal(err)
	}

	// Create app: slug only (product shell)
	form := url.Values{"slug": {"multiapp"}}
	req, _ := http.NewRequest("POST", ts.URL+"/api/apps", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body := new(bytes.Buffer)
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create app: %d %s", resp.StatusCode, body.String())
	}

	// Add the android platform (gets a generated signing key)
	form = url.Values{"platform": {"android"}, "packageName": {"io.multi"}}
	req, _ = http.NewRequest("POST", ts.URL+"/api/apps/multiapp/platforms", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body.Reset()
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 201 || !strings.Contains(body.String(), `"signingKey":"generated"`) {
		t.Fatalf("add android platform: %d %s", resp.StatusCode, body.String())
	}

	// Add the ios platform to the same slug
	form = url.Values{"platform": {"ios"}, "packageName": {"io.multi.ios"}}
	req, _ = http.NewRequest("POST", ts.URL+"/api/apps/multiapp/platforms", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body.Reset()
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("add platform: %d %s", resp.StatusCode, body.String())
	}

	// Listing shows both platforms under one slug
	req, _ = http.NewRequest("GET", ts.URL+"/api/apps", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body.Reset()
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	got := body.String()
	if !strings.Contains(got, `"slug":"multiapp"`) || !strings.Contains(got, `"platform":"android"`) || !strings.Contains(got, `"platform":"ios"`) {
		t.Fatalf("list apps missing platforms: %s", got)
	}

	// Android signing exists; ios signing 404s (no key for that platform)
	req, _ = http.NewRequest("GET", ts.URL+"/api/apps/multiapp/android/signing", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = ts.Client().Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("android signing: %v %v", err, resp.StatusCode)
	}
	resp.Body.Close()
	req, _ = http.NewRequest("GET", ts.URL+"/api/apps/multiapp/ios/signing", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = ts.Client().Do(req)
	if err != nil || resp.StatusCode != 404 {
		t.Fatalf("ios signing should 404: %v %v", err, resp.StatusCode)
	}
	resp.Body.Close()
}
