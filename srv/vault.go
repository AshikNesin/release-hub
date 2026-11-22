package srv

// 1Password vault for stored secrets (signing keystores, Play service
// accounts), through 1Password Connect: the same layout as tiny-agents
// (docs/features/vault.md and ADR 0027 there).
//
// A stored secret column holds either a sealed value ("aes:…" / "plain:…",
// secrets.go) or a reference to a field of a 1Password item:
//
//	op:<vaultID>/<itemID>#<fieldID>
//
// Fields whose id ends in "_base64" hold binary values (keystores) base64
// encoded; the rest are the value as text.
//
// Configuration (env):
//
//	OP_CONNECT_CREDENTIALS  1password-credentials.json (JSON or base64): the
//	                        image's bundled Connect server runs inside this
//	                        container (vault_bundled.go)
//	OP_CONNECT_HOST         or: a Connect server run elsewhere
//	OP_CONNECT_TOKEN        Connect access token (read + write) for one vault
//	OP_CONNECT_VAULT_ID     optional: the vault; unset, the one vault the token sees
//
// With Connect configured, every new key goes to 1Password; keys stored
// before stay sealed in the database until moved (vault_migrate.go).

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	opRefPrefix = "op:"
	// opTag marks every item release-hub creates; opRetiredTag is added when
	// release-hub stops using an item (key replaced, app deleted). Items are
	// never deleted from 1Password: a signing key is not recoverable.
	opTag        = "release-hub"
	opRetiredTag = "release-hub/retired"

	vaultGuardTTL      = 6 * time.Hour
	vaultCacheFresh    = 5 * time.Minute
	vaultColdRetry     = 8 * time.Second
	vaultRetryInterval = 500 * time.Millisecond
	vaultExpiryWarning = 14 * 24 * time.Hour
)

// vaultStatus is what the Settings page shows for 1Password.
type vaultStatus struct {
	State  string `json:"state"` // off | starting | ready | error
	Code   string `json:"code,omitempty"`
	Detail string `json:"detail"`
	Vault  string `json:"vault,omitempty"`
}

type vaultError struct {
	Code string // unreachable | unauthorized | not_found | overscoped | failed | missing_field
	Msg  string
}

func (e *vaultError) Error() string { return e.Msg }

func vaultErrCode(err error) string {
	var ve *vaultError
	if errors.As(err, &ve) {
		return ve.Code
	}
	return "failed"
}

// retryable is a failure worth waiting out (Connect starting or busy).
func retryable(err error) bool {
	switch vaultErrCode(err) {
	case "unreachable", "starting":
		return true
	}
	return false
}

type opField struct {
	ID      string `json:"id"`
	Type    string `json:"type,omitempty"`
	Purpose string `json:"purpose,omitempty"`
	Label   string `json:"label,omitempty"`
	Value   string `json:"value,omitempty"`
}

type opItem struct {
	ID       string    `json:"id,omitempty"`
	Title    string    `json:"title"`
	Category string    `json:"category"`
	Vault    opVaultID `json:"vault"`
	Tags     []string  `json:"tags,omitempty"`
	Fields   []opField `json:"fields,omitempty"`
}

type opVaultID struct {
	ID string `json:"id"`
}

type cachedSecret struct {
	value []byte
	at    time.Time
}

// opVault is a 1Password Connect client pinned to one vault.
type opVault struct {
	host    string
	token   string
	vaultID string // OP_CONNECT_VAULT_ID; "" = the one vault the token sees
	http    *http.Client
	bundled *bundledConnect // nil unless this container runs Connect itself
	now     func() time.Time

	mu      sync.Mutex
	guardAt time.Time
	guard   vaultStatus
	pinned  string
	cache   map[string]cachedSecret
}

// vaultFromEnv builds the vault from OP_CONNECT_* (nil when unconfigured)
// and starts the bundled Connect server when that is what the env asks for.
// dataDir holds the bundled server's state (the /data volume in Docker).
func vaultFromEnv(dataDir string) *opVault {
	creds := unquote(envStr("OP_CONNECT_CREDENTIALS"))
	host := unquote(envStr("OP_CONNECT_HOST"))
	if creds == "" && host == "" {
		return nil
	}
	v := newOpVault(host, unquote(envStr("OP_CONNECT_TOKEN")), unquote(envStr("OP_CONNECT_VAULT_ID")))
	if host == "" {
		v.host = bundledConnectHost
		v.bundled = startBundledConnectFromEnv(creds, dataDir)
	}
	return v
}

func newOpVault(host, token, vaultID string) *opVault {
	return &opVault{
		host:    strings.TrimRight(host, "/"),
		token:   token,
		vaultID: vaultID,
		http:    &http.Client{Timeout: 10 * time.Second},
		now:     time.Now,
		cache:   map[string]cachedSecret{},
	}
}

func unquote(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
	}
	return strings.TrimSpace(v)
}

func (v *opVault) call(ctx context.Context, method, path string, body, out any, step string) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, v.host+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+v.token)
	if body != nil {
		ct := "application/json"
		if method == http.MethodPatch {
			ct = "application/json-patch+json"
		}
		req.Header.Set("Content-Type", ct)
	}
	resp, err := v.http.Do(req)
	if err != nil {
		if v.bundled != nil {
			st := v.bundled.status()
			if st.State == "error" {
				return &vaultError{"unreachable", st.Detail}
			}
			return &vaultError{"starting", "The bundled 1Password Connect server is starting. " + st.Detail}
		}
		return &vaultError{"unreachable", "The 1Password Connect server is unreachable."}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return &vaultError{"unauthorized", "1Password Connect refused the token (expired, revoked or without access to the vault)."}
	case resp.StatusCode == 404 && step == "":
		return &vaultError{"not_found", "Item not found in 1Password."}
	case resp.StatusCode >= 500:
		return &vaultError{"unreachable", fmt.Sprintf("1Password Connect answered %d: %s", resp.StatusCode, connectMessage(raw, resp.Status))}
	case resp.StatusCode >= 300:
		what := "answered"
		if step != "" {
			what = "couldn't " + step + ":"
		}
		return &vaultError{"failed", fmt.Sprintf("1Password Connect %s %s (%d).", what, connectMessage(raw, resp.Status), resp.StatusCode)}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return &vaultError{"failed", "1Password Connect sent an unreadable answer."}
		}
	}
	return nil
}

func connectMessage(raw []byte, fallback string) string {
	var b struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &b) == nil && b.Message != "" {
		return b.Message
	}
	return fallback
}

// checkScope is the scoping guard: the token must see exactly the pinned
// vault, so a token that reaches other vaults is refused outright.
func (v *opVault) checkScope(ctx context.Context) (vaultStatus, string) {
	if v.token == "" {
		return vaultStatus{State: "error", Code: "unconfigured", Detail: "Set OP_CONNECT_TOKEN to a Connect access token for the vault."}, ""
	}
	var vaults []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := v.call(ctx, http.MethodGet, "/v1/vaults", nil, &vaults, ""); err != nil {
		state := "error"
		if vaultErrCode(err) == "starting" {
			state = "starting"
		}
		return vaultStatus{State: state, Code: vaultErrCode(err), Detail: err.Error()}, ""
	}
	pin := v.vaultID
	if pin == "" && len(vaults) == 1 {
		pin = vaults[0].ID
	}
	var name string
	var extra []string
	found := false
	for _, vt := range vaults {
		if vt.ID == pin {
			found, name = true, vt.Name
			continue
		}
		extra = append(extra, firstNonEmpty(vt.Name, vt.ID))
	}
	switch {
	case !found && v.vaultID != "":
		return vaultStatus{State: "error", Code: "overscoped", Detail: "The token cannot see the vault OP_CONNECT_VAULT_ID names."}, ""
	case !found:
		return vaultStatus{State: "error", Code: "overscoped", Detail: fmt.Sprintf("The token must see exactly one vault; it sees %d. Limit it to one vault or set OP_CONNECT_VAULT_ID.", len(vaults))}, ""
	case len(extra) > 0:
		return vaultStatus{State: "error", Code: "overscoped", Detail: "The token can see vaults besides the pinned one (" + strings.Join(extra, ", ") + "); every use is refused. Issue a token limited to one vault."}, ""
	}
	return vaultStatus{State: "ready", Detail: tokenExpiryNote(v.token, v.now()), Vault: firstNonEmpty(name, pin)}, pin
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// tokenExpiryNote names a token that expires within two weeks (Connect
// tokens are JWTs; an exp claim is optional).
func tokenExpiryNote(token string, now time.Time) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return ""
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Exp == 0 {
		return ""
	}
	left := time.Unix(claims.Exp, 0).Sub(now)
	switch {
	case left < 0:
		return "The token has expired."
	case left < vaultExpiryWarning:
		days := int(left.Hours() / 24)
		if days == 1 {
			return "The token expires in 1 day."
		}
		return fmt.Sprintf("The token expires in %d days.", days)
	}
	return ""
}

// status re-runs the guard when forced or when the last good check is
// older than vaultGuardTTL.
func (v *opVault) status(ctx context.Context, force bool) vaultStatus {
	v.mu.Lock()
	if !force && v.guard.State == "ready" && v.now().Sub(v.guardAt) < vaultGuardTTL {
		st := v.guard
		v.mu.Unlock()
		return st
	}
	v.mu.Unlock()
	st, pin := v.checkScope(ctx)
	v.mu.Lock()
	v.guard, v.guardAt, v.pinned = st, v.now(), pin
	v.mu.Unlock()
	return st
}

// ready returns the pinned vault id, or why the vault can't be used.
func (v *opVault) ready(ctx context.Context) (string, error) {
	st := v.status(ctx, false)
	if st.State != "ready" {
		code := firstNonEmpty(st.Code, st.State)
		return "", &vaultError{code, st.Detail}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.pinned, nil
}

func itemPath(vaultID, itemID string) string {
	p := "/v1/vaults/" + url.PathEscape(vaultID) + "/items"
	if itemID != "" {
		p += "/" + url.PathEscape(itemID)
	}
	return p
}

// vaultValue is one field written to an item. Binary values are stored
// base64 under an id ending in "_base64".
type vaultValue struct {
	ID        string
	Label     string
	Data      []byte
	Binary    bool
	Concealed bool
	// Info marks a readable copy for people opening the item (alias,
	// passwords, sha256). Nothing references it, so it is left out when
	// empty and not part of the read-back check.
	Info bool
}

func (f vaultValue) fieldID() string {
	if f.Binary && !strings.HasSuffix(f.ID, "_base64") {
		return f.ID + "_base64"
	}
	return f.ID
}

// createItem writes one item with the given fields, reads it back, checks
// every field round-tripped, and returns a reference per field (same order).
func (v *opVault) createItem(ctx context.Context, title, notes string, values []vaultValue) ([]string, error) {
	vaultID, err := v.ready(ctx)
	if err != nil {
		return nil, err
	}
	item := opItem{Title: title, Category: "API_CREDENTIAL", Vault: opVaultID{vaultID}, Tags: []string{opTag}}
	for _, f := range values {
		if f.Info && len(f.Data) == 0 {
			continue
		}
		typ := "STRING"
		if f.Concealed || f.Binary {
			typ = "CONCEALED"
		}
		val := string(f.Data)
		if f.Binary {
			val = base64.StdEncoding.EncodeToString(f.Data)
		}
		item.Fields = append(item.Fields, opField{ID: f.fieldID(), Type: typ, Label: f.Label, Value: val})
	}
	if notes != "" {
		item.Fields = append(item.Fields, opField{ID: "notesPlain", Type: "STRING", Purpose: "NOTES", Label: "notesPlain", Value: notes})
	}
	var created opItem
	if err := v.call(ctx, http.MethodPost, itemPath(vaultID, ""), item, &created, "save the item"); err != nil {
		return nil, err
	}
	if created.ID == "" {
		return nil, &vaultError{"failed", "1Password did not return the new item."}
	}
	refs := make([]string, len(values))
	for i, f := range values {
		refs[i] = opRefPrefix + vaultID + "/" + created.ID + "#" + f.fieldID()
	}
	// Read the copy back before anyone relies on it (Connect's sync can
	// answer a fresh item with 404 for a moment: fetch retries).
	for i, f := range values {
		if f.Info {
			continue
		}
		got, err := v.fetch(ctx, refs[i])
		if err != nil {
			v.retire(ctx, refs[i])
			return nil, fmt.Errorf("1Password saved the item but reading %s back failed: %w", f.fieldID(), err)
		}
		if !bytes.Equal(got, f.Data) {
			v.retire(ctx, refs[i])
			return nil, &vaultError{"failed", fmt.Sprintf("1Password saved the item but %s read back differently (%d bytes written, %d read).", f.fieldID(), len(f.Data), len(got))}
		}
		v.remember(refs[i], got)
	}
	return refs, nil
}

type opRef struct{ vault, item, field string }

func parseOpRef(ref string) (opRef, error) {
	rest, ok := strings.CutPrefix(ref, opRefPrefix)
	if !ok {
		return opRef{}, errors.New("not a 1Password reference")
	}
	path, field, ok := strings.Cut(rest, "#")
	vaultID, item, ok2 := strings.Cut(path, "/")
	if !ok || !ok2 || vaultID == "" || item == "" || field == "" {
		return opRef{}, errors.New("malformed 1Password reference")
	}
	return opRef{vaultID, item, field}, nil
}

var syncRetryDelays = []time.Duration{250 * time.Millisecond, 750 * time.Millisecond, 1500 * time.Millisecond}

func (v *opVault) getItem(ctx context.Context, vaultID, itemID string) (opItem, error) {
	var item opItem
	for attempt := 0; ; attempt++ {
		err := v.call(ctx, http.MethodGet, itemPath(vaultID, itemID), nil, &item, "")
		if err == nil {
			if item.Vault.ID != vaultID {
				return opItem{}, &vaultError{"not_found", "Item not found in the pinned vault."}
			}
			return item, nil
		}
		if vaultErrCode(err) != "not_found" || attempt >= len(syncRetryDelays) {
			return opItem{}, err
		}
		select {
		case <-ctx.Done():
			return opItem{}, err
		case <-time.After(syncRetryDelays[attempt]):
		}
	}
}

// fetch reads one referenced field from Connect (no cache).
func (v *opVault) fetch(ctx context.Context, ref string) ([]byte, error) {
	r, err := parseOpRef(ref)
	if err != nil {
		return nil, err
	}
	vaultID, err := v.ready(ctx)
	if err != nil {
		return nil, err
	}
	if r.vault != vaultID {
		return nil, &vaultError{"not_found", "The key lives in another 1Password vault than the one the token is pinned to."}
	}
	item, err := v.getItem(ctx, vaultID, r.item)
	if err != nil {
		return nil, err
	}
	for _, f := range item.Fields {
		if f.ID != r.field {
			continue
		}
		if strings.HasSuffix(r.field, "_base64") {
			b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(f.Value))
			if err != nil {
				return nil, &vaultError{"failed", "The 1Password field " + r.field + " is not valid base64."}
			}
			return b, nil
		}
		return []byte(f.Value), nil
	}
	return nil, &vaultError{"missing_field", fmt.Sprintf("The 1Password item %q has no field %q.", item.Title, r.field)}
}

func (v *opVault) remember(ref string, value []byte) {
	v.mu.Lock()
	v.cache[ref] = cachedSecret{value: value, at: v.now()}
	v.mu.Unlock()
}

// resolve returns a referenced value. A value read in the last five
// minutes is served from memory; when Connect fails, the last value read
// is served (a deploy restarts Connect while CI fetches keys). With none
// yet, a cold Connect gets a short retry window.
func (v *opVault) resolve(ctx context.Context, ref string) ([]byte, error) {
	v.mu.Lock()
	c, ok := v.cache[ref]
	v.mu.Unlock()
	if ok && v.now().Sub(c.at) < vaultCacheFresh {
		return c.value, nil
	}
	deadline := time.Now().Add(vaultColdRetry)
	for {
		val, err := v.fetch(ctx, ref)
		if err == nil {
			v.remember(ref, val)
			return val, nil
		}
		if ok {
			slog.Warn("1Password unavailable; serving the last value read", "error", err)
			return c.value, nil
		}
		if !retryable(err) || time.Now().Add(vaultRetryInterval).After(deadline) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(vaultRetryInterval):
		}
	}
}

// retire tags an item release-hub no longer uses. Best effort: a failure
// only leaves the tag off.
func (v *opVault) retire(ctx context.Context, ref string) {
	r, err := parseOpRef(ref)
	if err != nil {
		return
	}
	item, err := v.getItem(ctx, r.vault, r.item)
	if err != nil {
		slog.Warn("1Password item not tagged retired", "item", r.item, "error", err)
		return
	}
	for _, t := range item.Tags {
		if t == opRetiredTag {
			return
		}
	}
	patch := []map[string]any{{"op": "replace", "path": "/tags", "value": append(item.Tags, opRetiredTag)}}
	if err := v.call(ctx, http.MethodPatch, itemPath(r.vault, r.item), patch, nil, "tag the item"); err != nil {
		slog.Warn("1Password item not tagged retired", "item", r.item, "error", err)
	}
	v.mu.Lock()
	for k := range v.cache {
		if strings.HasPrefix(k, opRefPrefix+r.vault+"/"+r.item+"#") {
			delete(v.cache, k)
		}
	}
	v.mu.Unlock()
}
