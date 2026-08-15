package srv

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSigningRoundtrip(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	tok := "rh_signing_test"
	if _, err := s.DB.Exec("INSERT INTO api_tokens (name, token_hash) VALUES ('t', ?)", hashToken(tok)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO apps (slug, package_name, platform) VALUES ('sg', 'io.sg', 'android')"); err != nil {
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
