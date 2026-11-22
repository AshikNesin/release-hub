package srv

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeConnect is a 1Password Connect server: the network boundary the vault
// talks to. It implements the routes the vault uses.
type fakeConnect struct {
	t      *testing.T
	mu     sync.Mutex
	token  string
	vaults []map[string]string
	items  map[string]opItem
	next   int
	down   bool
	// dropEmpty mimics Connect leaving out fields saved with no value.
	dropEmpty bool
	ts        *httptest.Server
}

func newFakeConnect(t *testing.T, vaults ...string) *fakeConnect {
	fc := &fakeConnect{t: t, token: "op-token", items: map[string]opItem{}}
	for _, v := range vaults {
		fc.vaults = append(fc.vaults, map[string]string{"id": v, "name": "vault " + v})
	}
	fc.ts = httptest.NewServer(http.HandlerFunc(fc.serve))
	t.Cleanup(fc.ts.Close)
	return fc
}

func (fc *fakeConnect) serve(w http.ResponseWriter, r *http.Request) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.down {
		http.Error(w, `{"message":"down"}`, 503)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+fc.token {
		http.Error(w, `{"message":"bad token"}`, 401)
		return
	}
	segs := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.Method == "GET" && r.URL.Path == "/v1/vaults":
		_ = json.NewEncoder(w).Encode(fc.vaults)
	case len(segs) == 4 && segs[3] == "items" && r.Method == "POST":
		var it opItem
		_ = json.NewDecoder(r.Body).Decode(&it)
		if it.Vault.ID != segs[2] {
			http.Error(w, `{"message":"vault mismatch"}`, 400)
			return
		}
		if fc.dropEmpty {
			kept := it.Fields[:0]
			for _, f := range it.Fields {
				if f.Value != "" {
					kept = append(kept, f)
				}
			}
			it.Fields = kept
		}
		fc.next++
		it.ID = fmt.Sprintf("item%02d", fc.next)
		fc.items[it.ID] = it
		_ = json.NewEncoder(w).Encode(it)
	case len(segs) == 5 && segs[3] == "items":
		it, ok := fc.items[segs[4]]
		if !ok || it.Vault.ID != segs[2] {
			http.Error(w, `{"message":"not found"}`, 404)
			return
		}
		switch r.Method {
		case "GET":
			_ = json.NewEncoder(w).Encode(it)
		case "PATCH":
			var ops []struct {
				Op    string   `json:"op"`
				Path  string   `json:"path"`
				Value []string `json:"value"`
			}
			_ = json.NewDecoder(r.Body).Decode(&ops)
			for _, op := range ops {
				if op.Path == "/tags" {
					it.Tags = op.Value
				}
			}
			fc.items[it.ID] = it
			_ = json.NewEncoder(w).Encode(it)
		case "DELETE":
			fc.t.Errorf("release-hub must never delete a 1Password item")
			w.WriteHeader(204)
		}
	default:
		http.Error(w, `{"message":"no route"}`, 404)
	}
}

func (fc *fakeConnect) item(id string) opItem {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.items[id]
}

func (fc *fakeConnect) setDown(down bool) {
	fc.mu.Lock()
	fc.down = down
	fc.mu.Unlock()
}

func fieldValue(it opItem, id string) string {
	for _, f := range it.Fields {
		if f.ID == id {
			return f.Value
		}
	}
	return ""
}

func itemIDOf(t *testing.T, ref string) string {
	t.Helper()
	r, err := parseOpRef(ref)
	if err != nil {
		t.Fatalf("not a 1Password reference: %q", ref)
	}
	return r.item
}

func bearer(t *testing.T, s *Server) string {
	t.Helper()
	tok := "rh_vault_test"
	if _, err := s.DB.Exec("INSERT INTO api_tokens (name, token_hash) VALUES ('t', ?)", hashToken(tok)); err != nil {
		t.Fatal(err)
	}
	return tok
}

func apiDo(t *testing.T, ts *httptest.Server, tok, method, path, ctype string, body io.Reader) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, body)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func createAndroidApp(t *testing.T, ts *httptest.Server, tok, slug string) {
	t.Helper()
	form := "application/x-www-form-urlencoded"
	if resp, b := apiDo(t, ts, tok, "POST", "/api/apps", form, strings.NewReader("slug="+slug)); resp.StatusCode != 201 {
		t.Fatalf("create app: %d %s", resp.StatusCode, b)
	}
	resp, b := apiDo(t, ts, tok, "POST", "/api/apps/"+slug+"/platforms", form, strings.NewReader("platform=android&packageName=io."+slug))
	if resp.StatusCode != 201 || !strings.Contains(string(b), `"signingKey":"generated"`) {
		t.Fatalf("add platform: %d %s", resp.StatusCode, b)
	}
}

func uploadPlayAccount(t *testing.T, ts *httptest.Server, tok string, creds []byte) {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("file", "sa.json")
	fw.Write(creds)
	mw.Close()
	if resp, b := apiDo(t, ts, tok, "POST", "/api/play-accounts", mw.FormDataContentType(), body); resp.StatusCode != 200 {
		t.Fatalf("play account: %d %s", resp.StatusCode, b)
	}
}

type fetchedKey struct {
	ks                         []byte
	sha, storePW, alias, keyPW string
}

func fetchSigning(t *testing.T, ts *httptest.Server, tok, slug string) fetchedKey {
	t.Helper()
	resp, b := apiDo(t, ts, tok, "GET", "/api/apps/"+slug+"/signing", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("fetch signing: %d %s", resp.StatusCode, b)
	}
	return fetchedKey{b, resp.Header.Get("X-Release-Hub-Keystore-Sha256"), resp.Header.Get("X-Release-Hub-Store-Password"),
		resp.Header.Get("X-Release-Hub-Key-Alias"), resp.Header.Get("X-Release-Hub-Key-Password")}
}

func storedSigning(t *testing.T, s *Server, slug string) (ks, cfg string) {
	t.Helper()
	if err := s.DB.QueryRow(`SELECT p.sign_keystore, p.sign_config FROM app_platforms p JOIN apps a ON a.id = p.app_id WHERE a.slug = ?`, slug).Scan(&ks, &cfg); err != nil {
		t.Fatal(err)
	}
	return
}

func TestVaultRefusesTokenThatSeesOtherVaults(t *testing.T) {
	fc := newFakeConnect(t, "vlt1", "vlt2")
	v := newOpVault(fc.ts.URL, fc.token, "vlt1")
	st := v.status(t.Context(), true)
	if st.State != "error" || st.Code != "overscoped" || !strings.Contains(st.Detail, "vault vlt2") {
		t.Fatalf("status = %+v, want overscoped naming vault vlt2", st)
	}
	if _, err := v.createItem(t.Context(), "x", "", []vaultValue{{ID: "a", Data: []byte("b")}}); err == nil {
		t.Fatal("createItem on an overscoped token must fail")
	}

	unpinned := newOpVault(fc.ts.URL, fc.token, "")
	if st := unpinned.status(t.Context(), true); st.Code != "overscoped" || !strings.Contains(st.Detail, "sees 2") {
		t.Fatalf("unpinned status = %+v, want 'must see exactly one vault; it sees 2'", st)
	}

	single := newFakeConnect(t, "only")
	ok := newOpVault(single.ts.URL, single.token, "")
	if st := ok.status(t.Context(), true); st.State != "ready" || st.Vault != "vault only" {
		t.Fatalf("single-vault status = %+v, want ready on vault only", st)
	}
}

// With Connect configured, a generated keystore and an uploaded Play key go
// to 1Password; the database keeps only references, and CI fetches the
// same bytes as before.
func TestNewKeysAreStoredInOnePassword(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "")
	fc := newFakeConnect(t, "vlt1")
	s, ts := newTestServer(t)
	s.vault = newOpVault(fc.ts.URL, fc.token, "")
	tok := bearer(t, s)

	createAndroidApp(t, ts, tok, "demo")
	ksRef, cfgRef := storedSigning(t, s, "demo")
	if !strings.HasPrefix(ksRef, "op:vlt1/") || !strings.HasSuffix(ksRef, "#keystore_base64") || !strings.HasSuffix(cfgRef, "#signing_config") {
		t.Fatalf("stored = %q / %q, want 1Password references", ksRef, cfgRef)
	}
	item := fc.item(itemIDOf(t, ksRef))
	if item.Title != "release-hub: demo (android) signing key" || item.Category != "API_CREDENTIAL" || len(item.Tags) != 1 || item.Tags[0] != "release-hub" {
		t.Fatalf("item = %+v", item)
	}

	got := fetchSigning(t, ts, tok, "demo")
	sum := sha256.Sum256(got.ks)
	if got.sha != hex.EncodeToString(sum[:]) || got.alias != "release" || got.storePW == "" || got.keyPW != got.storePW {
		t.Fatalf("fetched = sha %q alias %q", got.sha, got.alias)
	}
	if fieldValue(item, "keystore_sha256") != got.sha || fieldValue(item, "key_alias") != "release" || fieldValue(item, "store_password") != got.storePW {
		t.Fatal("the item's readable fields don't match what CI fetched")
	}

	uploadPlayAccount(t, ts, tok, []byte(`{"client_email":"pub@x.iam.gserviceaccount.com","private_key":"k"}`))
	var creds string
	_ = s.DB.QueryRow(`SELECT credentials FROM play_accounts`).Scan(&creds)
	if !strings.HasSuffix(creds, "#service_account_json") {
		t.Fatalf("play credentials stored as %q, want a 1Password reference", creds)
	}
	resp, b := apiDo(t, ts, tok, "GET", "/api/play-accounts", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(string(b), "pub@x.iam.gserviceaccount.com") {
		t.Fatalf("list play accounts: %s", b)
	}
}

// Keys stored in the database before 1Password was set up move into it,
// and everything that reads them keeps working with the same values.
func TestMigrateMovesStoredKeysIntoOnePassword(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	tok := bearer(t, s)
	createAndroidApp(t, ts, tok, "one")
	createAndroidApp(t, ts, tok, "two")
	saJSON := []byte(`{"client_email":"pub@x.iam.gserviceaccount.com","private_key":"k"}`)
	uploadPlayAccount(t, ts, tok, saJSON)
	before := fetchSigning(t, ts, tok, "one")
	if ks, _ := storedSigning(t, s, "one"); !strings.HasPrefix(ks, "aes:") {
		t.Fatalf("precondition: keystore stored as %q", ks[:4])
	}

	resp, b := apiDo(t, ts, tok, "POST", "/api/vault/migrate", "", nil)
	if resp.StatusCode != 400 || !strings.Contains(string(b), "not configured") {
		t.Fatalf("migrate without 1Password: %d %s", resp.StatusCode, b)
	}

	fc := newFakeConnect(t, "vlt1")
	s.vault = newOpVault(fc.ts.URL, fc.token, "")
	resp, b = apiDo(t, ts, tok, "GET", "/api/vault", "", nil)
	var info vaultInfo
	_ = json.Unmarshal(b, &info)
	if resp.StatusCode != 200 || info.Status.State != "ready" || info.Local != (vaultCounts{2, 1}) || info.InVault != (vaultCounts{}) {
		t.Fatalf("status before move: %s", b)
	}

	resp, b = apiDo(t, ts, tok, "POST", "/api/vault/migrate", "", nil)
	var res vaultMoveResult
	_ = json.Unmarshal(b, &res)
	want := []string{"one/android signing key", "two/android signing key", "Play service account pub@x.iam.gserviceaccount.com"}
	if resp.StatusCode != 200 || len(res.Failed) != 0 || strings.Join(res.Moved, "|") != strings.Join(want, "|") {
		t.Fatalf("migrate: %d %s", resp.StatusCode, b)
	}

	ks, cfg := storedSigning(t, s, "one")
	if !strings.HasPrefix(ks, "op:vlt1/") || !strings.HasPrefix(cfg, "op:vlt1/") {
		t.Fatalf("after move: %q / %q", ks, cfg)
	}
	var creds string
	_ = s.DB.QueryRow(`SELECT credentials FROM play_accounts`).Scan(&creds)
	if !strings.HasPrefix(creds, "op:vlt1/") || fieldValue(fc.item(itemIDOf(t, creds)), "service_account_json") != string(saJSON) {
		t.Fatalf("play account after move: %q", creds)
	}

	// Moved values are what 1Password serves, not a cache of the old rows.
	s.vault = newOpVault(fc.ts.URL, fc.token, "")
	after := fetchSigning(t, ts, tok, "one")
	if !bytes.Equal(after.ks, before.ks) || after.sha != before.sha || after.storePW != before.storePW || after.alias != before.alias || after.keyPW != before.keyPW {
		t.Fatal("the keystore served after the move differs from the one before it")
	}

	resp, b = apiDo(t, ts, tok, "POST", "/api/vault/migrate", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(string(b), `"moved":[]`) {
		t.Fatalf("second migrate should move nothing: %s", b)
	}
	_, b = apiDo(t, ts, tok, "GET", "/api/vault", "", nil)
	info = vaultInfo{}
	_ = json.Unmarshal(b, &info)
	if info.Local != (vaultCounts{}) || info.InVault != (vaultCounts{2, 1}) {
		t.Fatalf("status after move: %s", b)
	}
}

func TestMigrateLeavesUnreadableKeyInPlace(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	tok := bearer(t, s)
	createAndroidApp(t, ts, tok, "one")
	if _, err := s.DB.Exec(`UPDATE app_platforms SET sign_sha256 = 'deadbeef'`); err != nil {
		t.Fatal(err)
	}
	ksBefore, _ := storedSigning(t, s, "one")
	fc := newFakeConnect(t, "vlt1")
	s.vault = newOpVault(fc.ts.URL, fc.token, "")

	_, b := apiDo(t, ts, tok, "POST", "/api/vault/migrate", "", nil)
	var res vaultMoveResult
	_ = json.Unmarshal(b, &res)
	if len(res.Moved) != 0 || len(res.Failed) != 1 || res.Failed[0].Name != "one/android" || !strings.Contains(res.Failed[0].Reason, "sha256") {
		t.Fatalf("migrate: %s", b)
	}
	if ks, _ := storedSigning(t, s, "one"); ks != ksBefore {
		t.Fatal("a key that failed to move must stay as it was")
	}
}

func TestVaultServesLastValueWhileConnectIsDown(t *testing.T) {
	fc := newFakeConnect(t, "vlt1")
	v := newOpVault(fc.ts.URL, fc.token, "")
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	v.now = func() time.Time { return now }
	refs, err := v.createItem(t.Context(), "k", "", []vaultValue{{ID: "keystore", Data: []byte{0, 1, 2, 255}, Binary: true}})
	if err != nil {
		t.Fatal(err)
	}
	fc.setDown(true)
	now = now.Add(time.Hour)
	got, err := v.resolve(t.Context(), refs[0])
	if err != nil || !bytes.Equal(got, []byte{0, 1, 2, 255}) {
		t.Fatalf("resolve while down = %v, %v; want the last value read", got, err)
	}

	cold := newOpVault(fc.ts.URL, fc.token, "")
	start := time.Now()
	if _, err := cold.resolve(t.Context(), refs[0]); err == nil {
		t.Fatal("a value never read can't be served while Connect is down")
	}
	if waited := time.Since(start); waited < 5*time.Second || waited > 12*time.Second {
		t.Fatalf("cold resolve gave up after %v, want the ~8 s retry window", waited)
	}
}

// Replacing or removing a key never deletes it from 1Password: the item is
// tagged retired (the fake fails the test on any DELETE).
func TestRemovedKeysAreTaggedRetiredNotDeleted(t *testing.T) {
	fc := newFakeConnect(t, "vlt1")
	s, ts := newTestServer(t)
	s.vault = newOpVault(fc.ts.URL, fc.token, "")
	tok := bearer(t, s)
	createAndroidApp(t, ts, tok, "demo")
	ksRef, _ := storedSigning(t, s, "demo")

	if resp, b := apiDo(t, ts, tok, "POST", "/api/apps/demo/signing/delete", "", nil); resp.StatusCode != 200 {
		t.Fatalf("delete signing: %d %s", resp.StatusCode, b)
	}
	tags := fc.item(itemIDOf(t, ksRef)).Tags
	if strings.Join(tags, ",") != "release-hub,release-hub/retired" {
		t.Fatalf("tags after delete = %v", tags)
	}
	if resp, _ := apiDo(t, ts, tok, "GET", "/api/apps/demo/signing", "", nil); resp.StatusCode != 404 {
		t.Fatalf("signing after delete: %d, want 404", resp.StatusCode)
	}
}

func TestSessionFromCredentials(t *testing.T) {
	js := `{"verifier":{"salt":"s"},"deviceUuid":"dev","uniqueKey":{}}`
	want := "eyJ2ZXJpZmllciI6eyJzYWx0IjoicyJ9LCJkZXZpY2VVdWlkIjoiZGV2IiwidW5pcXVlS2V5Ijp7fX0="
	for _, in := range []string{js, " '" + js + "' ", want} {
		got, err := sessionFromCredentials(in)
		if err != nil || got != want {
			t.Fatalf("sessionFromCredentials(%.20q) = %q, %v", in, got, err)
		}
	}
	if _, err := sessionFromCredentials(`{"not":"credentials"}`); err == nil {
		t.Fatal("JSON without deviceUuid must be refused")
	}
}

// The bundled server runs both Connect processes with the session and the
// loopback bus wiring, and restarts one that exits.
func TestBundledConnectRunsAndRestartsBothProcesses(t *testing.T) {
	bin := t.TempDir()
	logDir := t.TempDir()
	for _, name := range []string{"connect-api", "connect-sync"} {
		script := "#!/bin/sh\necho \"$OP_SESSION $OP_HTTP_PORT $OP_BUS_PORT $OP_BUS_PEERS $HOME\" >> " + filepath.Join(logDir, name) + "\n"
		if name == "connect-api" {
			script += "exit 3\n"
		} else {
			script += "exec sleep 30\n"
		}
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	b := startBundledConnect(bundledOptions{session: "SESSION", dir: dir, binDir: bin, ports: bundledPorts{1, 2, 3, 4}, backoff: 50 * time.Millisecond})
	t.Cleanup(b.stop)

	deadline := time.Now().Add(5 * time.Second)
	var api []byte
	for time.Now().Before(deadline) {
		api, _ = os.ReadFile(filepath.Join(logDir, "connect-api"))
		if strings.Count(string(api), "\n") >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	home := filepath.Join(dir, "home")
	if lines := strings.Split(strings.TrimSpace(string(api)), "\n"); len(lines) < 3 || lines[0] != "SESSION 1 3 localhost:4 "+home {
		t.Fatalf("connect-api runs = %q, want ≥3 restarts with session, port 1, bus 3 peering 4", api)
	}
	sync, _ := os.ReadFile(filepath.Join(logDir, "connect-sync"))
	if strings.TrimSpace(string(sync)) != "SESSION 2 4 localhost:3 "+home {
		t.Fatalf("connect-sync runs = %q, want one run on port 2, bus 4 peering 3", sync)
	}
	b.stop()
	if st := b.status(); st.State == "ready" && len(b.running) != 0 {
		t.Fatal("stop left processes running")
	}
}

func TestBundledConnectWithoutBinariesSaysWhy(t *testing.T) {
	b := startBundledConnect(bundledOptions{session: "S", dir: t.TempDir(), binDir: t.TempDir(), ports: defaultBundledPorts, backoff: time.Second})
	if st := b.status(); st.State != "error" || !strings.Contains(st.Detail, "connect-sync is not in this image") {
		t.Fatalf("status = %+v", st)
	}
}

// A keystore whose readable copies are empty (e.g. no key password stored)
// still moves: Connect may drop empty fields, and only the referenced
// fields are read back.
func TestMigrateMovesKeyWithEmptyReadableFields(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	tok := bearer(t, s)
	createAndroidApp(t, ts, tok, "one")
	cfg, _ := encryptCreds([]byte(`{"storePassword":"pw","keyAlias":"release","keyPassword":""}`))
	if _, err := s.DB.Exec(`UPDATE app_platforms SET sign_config = ?`, cfg); err != nil {
		t.Fatal(err)
	}
	fc := newFakeConnect(t, "vlt1")
	fc.dropEmpty = true
	s.vault = newOpVault(fc.ts.URL, fc.token, "")

	_, b := apiDo(t, ts, tok, "POST", "/api/vault/migrate", "", nil)
	var res vaultMoveResult
	_ = json.Unmarshal(b, &res)
	if len(res.Moved) != 1 || len(res.Failed) != 0 {
		t.Fatalf("migrate: %s", b)
	}
	if got := fetchSigning(t, ts, tok, "one"); got.storePW != "pw" || got.keyPW != "" {
		t.Fatalf("after move: store %q key %q", got.storePW, got.keyPW)
	}
}

// Settings lists each key still in the database with why it stayed.
func TestVaultStatusSaysWhyKeysStayed(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	tok := bearer(t, s)
	createAndroidApp(t, ts, tok, "bad")
	createAndroidApp(t, ts, tok, "good")
	if _, err := s.DB.Exec(`UPDATE app_platforms SET sign_sha256 = 'deadbeef' WHERE app_id = (SELECT id FROM apps WHERE slug = 'bad')`); err != nil {
		t.Fatal(err)
	}
	fc := newFakeConnect(t, "vlt1")
	s.vault = newOpVault(fc.ts.URL, fc.token, "")
	apiDo(t, ts, tok, "POST", "/api/vault/migrate", "", nil)

	_, b := apiDo(t, ts, tok, "GET", "/api/vault", "", nil)
	var info vaultInfo
	_ = json.Unmarshal(b, &info)
	if len(info.Pending) != 1 || info.Pending[0].Name != "bad/android" || !strings.Contains(info.Pending[0].Reason, "sha256") {
		t.Fatalf("pending = %s", b)
	}
}

// Platforms left behind by apps deleted before foreign keys were on for
// every connection hold keys no app uses: they are not "in the database"
// waiting to move, which the move would never pick up.
func TestVaultCountsSkipKeysOfDeletedApps(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	tok := bearer(t, s)
	createAndroidApp(t, ts, tok, "kept")
	createAndroidApp(t, ts, tok, "gone")
	if _, err := s.DB.Exec(`PRAGMA foreign_keys = OFF; DELETE FROM apps WHERE slug = 'gone'; PRAGMA foreign_keys = ON;`); err != nil {
		t.Fatal(err)
	}
	_, b := apiDo(t, ts, tok, "GET", "/api/vault", "", nil)
	var info vaultInfo
	_ = json.Unmarshal(b, &info)
	if info.Local.SigningKeys != 1 || info.OrphanedKeys != 1 || len(info.Pending) != 1 {
		t.Fatalf("status = %s", b)
	}
}
