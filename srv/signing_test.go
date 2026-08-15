package srv

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	pkcs12 "software.sslmate.com/src/go-pkcs12"

	"srv.exe.dev/db/dbgen"
)

func TestSigningRoundtrip(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	tok := "rh_signing_test"
	if _, err := s.DB.Exec("INSERT INTO api_tokens (name, token_hash) VALUES ('t', ?)", hashToken(tok)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO apps (slug) VALUES ('sg')"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO app_platforms (app_id, platform, package_name) VALUES (1, 'android', 'io.sg')"); err != nil {
		t.Fatal(err)
	}

	// store
	ksBytes := []byte("FAKE-JKS-BYTES")
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("file", "release.jks")
	fw.Write(ksBytes)
	mw.WriteField("storePassword", "s3cret")
	mw.WriteField("keyAlias", "release")
	mw.Close()
	req, _ := http.NewRequest("POST", ts.URL+"/api/apps/sg/signing", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body2 := new(bytes.Buffer)
	body2.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("store signing: %d body=%s", resp.StatusCode, body2.String())
	}

	// fetch and compare
	req, _ = http.NewRequest("GET", ts.URL+"/api/apps/sg/signing", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("fetch signing: %d", resp.StatusCode)
	}
	got := new(bytes.Buffer)
	got.ReadFrom(resp.Body)
	if !bytes.Equal(got.Bytes(), ksBytes) {
		t.Fatal("keystore bytes mismatch")
	}
	sum := sha256.Sum256(ksBytes)
	if resp.Header.Get("X-Hub-Keystore-Sha256") != hex.EncodeToString(sum[:]) {
		t.Fatal("sha header mismatch")
	}
	if resp.Header.Get("X-Hub-Store-Password") != "s3cret" || resp.Header.Get("X-Hub-Key-Alias") != "release" {
		t.Fatal("config headers mismatch")
	}

	// unauthenticated access must fail (secrets!)
	req, _ = http.NewRequest("GET", ts.URL+"/api/apps/sg/signing", nil)
	resp2 := httptest.NewRecorder()
	s.requireAPI(s.handleApiGetSigning)(resp2, req)
	if resp2.Code != 401 {
		t.Fatalf("expected 401 without token, got %d", resp2.Code)
	}
}

// Creating an android app must auto-generate a usable signing keystore:
// fetch it back, decrypt, and verify the RSA key + cert actually parse.
func TestCreateAppGeneratesSigning(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	tok := "rh_signing_gen_test"
	if _, err := s.DB.Exec("INSERT INTO api_tokens (name, token_hash) VALUES ('t', ?)", hashToken(tok)); err != nil {
		t.Fatal(err)
	}

	form := url.Values{"slug": {"genapp"}}
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
		t.Fatalf("create app: %d body=%s", resp.StatusCode, body.String())
	}
	form = url.Values{"platform": {"android"}, "packageName": {"io.genapp"}}
	req, _ = http.NewRequest("POST", ts.URL+"/api/apps/genapp/platforms", strings.NewReader(form.Encode()))
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
		t.Fatalf("add android platform: %d body=%s", resp.StatusCode, body.String())
	}

	// Fetch the keystore + password headers and validate the PKCS#12 parses.
	req, _ = http.NewRequest("GET", ts.URL+"/api/apps/genapp/signing", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("fetch signing: %d", resp.StatusCode)
	}
	pw := resp.Header.Get("X-Hub-Store-Password")
	if len(pw) < 24 {
		t.Fatalf("expected high-entropy store password, got %q", pw)
	}
	p12 := new(bytes.Buffer)
	p12.ReadFrom(resp.Body)
	if p12.Len() < 1000 {
		t.Fatalf("keystore suspiciously small: %d bytes", p12.Len())
	}
	key, cert, err := pkcs12.Decode(p12.Bytes(), pw)
	if err != nil {
		t.Fatalf("decode generated pkcs12: %v", err)
	}
	if _, ok := key.(*rsa.PrivateKey); !ok {
		t.Fatalf("expected RSA key, got %T", key)
	}
	if cert.Subject.CommonName != "Android App: genapp" {
		t.Fatalf("unexpected cert CN: %q", cert.Subject.CommonName)
	}
	if time.Until(cert.NotAfter) < 29*365*24*time.Hour {
		t.Fatalf("cert validity too short: expires %s", cert.NotAfter)
	}
}

// Settings (sign_org etc.) must flow into the generated certificate's DN.
func TestGeneratedKeyUsesConfiguredSubject(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, _ := newTestServer(t)
	q := dbgen.New(s.DB)
	ctx := context.Background()
	for k, v := range map[string]string{
		"sign_org":      "Nesin Technologies",
		"sign_ou":       "Mobile",
		"sign_locality": "Chennai",
		"sign_state":    "Tamil Nadu",
		"sign_country":  "IN",
	} {
		if err := q.SetConfig(ctx, dbgen.SetConfigParams{Key: k, Value: v}); err != nil {
			t.Fatal(err)
		}
	}
	sub := s.subjectFromConfig(ctx)
	if sub.Organization != "Nesin Technologies" || sub.Country != "IN" {
		t.Fatalf("subject not loaded from config: %+v", sub)
	}
	name := sub.pkixName("Android App: x")
	if name.CommonName != "Android App: x" ||
		name.Organization[0] != "Nesin Technologies" ||
		name.OrganizationalUnit[0] != "Mobile" ||
		name.Locality[0] != "Chennai" ||
		name.Province[0] != "Tamil Nadu" ||
		name.Country[0] != "IN" {
		t.Fatalf("unexpected DN: %+v", name)
	}
	// And the end-to-end path: generate with this subject and read the cert back.
	p12, cfg, _, err := generateKeystoreWithSubject("Android App: x", sub)
	if err != nil {
		t.Fatal(err)
	}
	_, cert, err := pkcs12.Decode(p12, cfg.StorePassword)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.Organization[0] != "Nesin Technologies" || cert.Subject.Country[0] != "IN" {
		t.Fatalf("cert subject wrong: %+v", cert.Subject)
	}
}
