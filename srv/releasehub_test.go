package srv

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

var lastTestServer *Server

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.sqlite3"), "test-host")
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	s.BaseURL = "http://hub.test"
	s.Storage = &LocalStorage{Dir: filepath.Join(dir, "artifacts"), BaseURL: "http://hub.test"}
	lastTestServer = s
	ts := httptest.NewServer(s.muxForTest())
	t.Cleanup(ts.Close)
	return s, ts
}

// muxForTest builds the same route table as Serve without listening.
func (s *Server) muxForTest() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("GET /setup", s.handleFirstRun)
	m.HandleFunc("POST /setup", s.handleFirstRun)
	m.HandleFunc("GET /login", s.handleLogin)
	m.HandleFunc("POST /login", s.handleLogin)
	m.HandleFunc("GET /logout", s.handleLogout)
	m.HandleFunc("GET /{$}", s.requireUI(s.handleHome))
	m.HandleFunc("POST /apps", s.requireUI(s.handleCreateAppUI))
	m.HandleFunc("POST /tokens", s.requireUI(s.handleCreateTokenUI))
	m.HandleFunc("GET /apps/{slug}", s.requireUI(s.handleAppDetail))
	m.HandleFunc("GET /api/apps", s.requireAPI(s.handleApiListApps))
	m.HandleFunc("POST /api/apps", s.requireAPI(s.handleApiCreateApp))
	m.HandleFunc("POST /api/apps/{slug}/platforms", s.requireAPI(s.handleApiAddPlatform))
	m.HandleFunc("POST /api/apps/{slug}/releases", s.requireAPI(s.handleApiUpload))
	m.HandleFunc("POST /api/apps/{slug}/{platform}/releases", s.requireAPI(s.handleApiUpload))
	m.HandleFunc("GET /api/apps/{slug}/releases", s.requireAPI(s.handleApiReleases))
	m.HandleFunc("POST /api/tokens", s.requireAPI(s.handleApiCreateToken))
	m.HandleFunc("POST /api/apps/{slug}/play", s.requireAPI(s.handleApiSetPlay))
	m.HandleFunc("POST /api/apps/{slug}/signing", s.requireAPI(s.handleApiSetSigning))
	m.HandleFunc("GET /api/apps/{slug}/signing", s.requireAPI(s.handleApiGetSigning))
	m.HandleFunc("POST /api/apps/{slug}/signing/delete", s.requireAPI(s.handleApiDeleteSigning))
	m.HandleFunc("GET /api/apps/{slug}/manifest", s.handleManifest)
	m.HandleFunc("GET /api/apps/{slug}/{platform}/signing", s.requireAPI(s.handleApiGetSigning))
	m.HandleFunc("GET /artifacts/{slug}/{platform}/{file}", s.handleArtifact)
	return m
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
	_, ts := newTestServer(t)
	client := ts.Client()

	// Seed a bearer token directly in the DB with a known hash.
	tok := "rh_testtoken123"
	if _, err := lastTestServer.DB.Exec(
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



// App → platforms: one slug, android + ios variants, independent releases
// and signing keys per platform.
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
	body.Reset(); body.ReadFrom(resp.Body); resp.Body.Close()
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
	body.Reset(); body.ReadFrom(resp.Body); resp.Body.Close()
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
	body.Reset(); body.ReadFrom(resp.Body); resp.Body.Close()
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
