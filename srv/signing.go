package srv

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"fmt"
	"net/http"
	"os"

	"srv.exe.dev/db/dbgen"
)

// signingConfig is the plaintext structure stored encrypted in the DB.
type signingConfig struct {
	StorePassword string `json:"storePassword"`
	KeyAlias      string `json:"keyAlias"`
	KeyPassword   string `json:"keyPassword"`
}

// handleApiSetSigning POST /api/apps/{slug}/signing
// multipart: file=<keystore> storePassword= keyAlias= [keyPassword=]
// Stores the keystore encrypted; sha256 recorded in plaintext so gradle
// builds can verify what they fetched. keyPassword defaults to storePassword.
func (s *Server) handleApiSetSigning(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil || hdr.Size == 0 {
		writeErr(w, 400, "multipart file field with keystore required")
		return
	}
	defer f.Close()
	ks, err := io.ReadAll(io.LimitReader(f, 8<<20)) // keystores are small
	if err != nil {
		writeErr(w, 400, "read keystore: "+err.Error())
		return
	}
	cfg := signingConfig{
		StorePassword: r.FormValue("storePassword"),
		KeyAlias:      r.FormValue("keyAlias"),
		KeyPassword:   r.FormValue("keyPassword"),
	}
	if cfg.StorePassword == "" || cfg.KeyAlias == "" {
		writeErr(w, 400, "storePassword and keyAlias required")
		return
	}
	if cfg.KeyPassword == "" {
		cfg.KeyPassword = cfg.StorePassword
	}
	cfgJSON, _ := json.Marshal(cfg)
	encKS, err := encryptCreds(ks)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	encCfg, err := encryptCreds(cfgJSON)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	sum := sha256.Sum256(ks)
	if err := dbgen.New(s.DB).SetSigningConfig(r.Context(), dbgen.SetSigningConfigParams{
		SignKeystore: encKS, SignConfig: encCfg,
		SignSha256: hex.EncodeToString(sum[:]), ID: app.ID,
	}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"stored": true, "keystoreSha256": hex.EncodeToString(sum[:])})
}

// handleApiGetSigning GET /api/apps/{slug}/signing
// Returns the keystore bytes (X-Hub-Keystore-Sha256 header) and config
// headers for CI: any authenticated build env can produce signed releases.
// It is a bearer-auth endpoint — never expose it publicly.
func (s *Server) handleApiGetSigning(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	if app.SignKeystore == "" {
		writeErr(w, 404, "no signing keystore stored for this app")
		return
	}
	ks, err := decryptCreds(app.SignKeystore)
	if err != nil {
		writeErr(w, 500, "decrypt keystore: "+err.Error())
		return
	}
	cfgRaw, err := decryptCreds(app.SignConfig)
	if err != nil {
		writeErr(w, 500, "decrypt config: "+err.Error())
		return
	}
	var cfg signingConfig
	_ = json.Unmarshal(cfgRaw, &cfg)
	sum := sha256.Sum256(ks)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Hub-Keystore-Sha256", hex.EncodeToString(sum[:]))
	w.Header().Set("X-Hub-Store-Password", cfg.StorePassword)
	w.Header().Set("X-Hub-Key-Alias", cfg.KeyAlias)
	w.Header().Set("X-Hub-Key-Password", cfg.KeyPassword)
	w.WriteHeader(200)
	_, _ = w.Write(ks)
}

// handleApiDeleteSigning POST /api/apps/{slug}/signing/delete — wipes stored key material.
func (s *Server) handleApiDeleteSigning(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	if err := dbgen.New(s.DB).SetSigningConfig(r.Context(), dbgen.SetSigningConfigParams{
		SignKeystore: "", SignConfig: "", SignSha256: "", ID: app.ID,
	}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"stored": false})
}

// FetchSigningForBuild is the helper CI scripts use: writes the keystore to
// path and returns the signing properties for gradle.
func FetchSigningForBuild(hubBaseURL, slug, token, outPath string) (storePassword, keyAlias, keyPassword, sha string, err error) {
	req, err := http.NewRequest("GET", hubBaseURL+"/api/apps/"+slug+"/signing", nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = fmt.Errorf("fetch signing: %s", resp.Status)
		return
	}
	ks, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	if err = os.WriteFile(outPath, ks, 0o600); err != nil {
		return
	}
	storePassword = resp.Header.Get("X-Hub-Store-Password")
	keyAlias = resp.Header.Get("X-Hub-Key-Alias")
	keyPassword = resp.Header.Get("X-Hub-Key-Password")
	sha = resp.Header.Get("X-Hub-Keystore-Sha256")
	return
}
