package srv

// Moving keys stored in the database into 1Password, and the vault's
// status for the Settings page.
//
//	GET  /api/v1/vault           (UI)     GET  /api/vault           (bearer)
//	POST /api/v1/vault/migrate   (UI)     POST /api/vault/migrate   (bearer)
//
// A move copies each sealed value into a 1Password item, reads the copy
// back and checks it, then points the row at the copy in one guarded
// UPDATE (the row must still hold what was copied). A key that fails stays
// in the database and the answer says why. Running it again moves only
// what is left.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

// vaultMoves serialises moves (two at once would copy a key twice) and
// remembers why keys failed, so Settings can show it after the toast.
type vaultMoves struct {
	run    sync.Mutex
	mu     sync.Mutex
	failed map[string]vaultMoveFailure // by kind + name
}

func (m *vaultMoves) record(res vaultMoveResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failed = map[string]vaultMoveFailure{}
	for _, f := range res.Failed {
		m.failed[f.Kind+":"+f.Name] = f
	}
}

func (m *vaultMoves) lastFailure(kind, name string) (vaultMoveFailure, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.failed[kind+":"+name]
	return f, ok
}

type vaultCounts struct {
	SigningKeys  int `json:"signingKeys"`
	PlayAccounts int `json:"playAccounts"`
}

type vaultInfo struct {
	Configured bool        `json:"configured"`
	Mode       string      `json:"mode"` // off | bundled | external
	Status     vaultStatus `json:"status"`
	Local      vaultCounts `json:"local"`
	InVault    vaultCounts `json:"inVault"`
	// Pending lists the keys still in the database, with why the last
	// move left each one there (empty when it wasn't tried yet).
	Pending []vaultPending `json:"pending"`
	// OrphanedKeys counts signing keys left by apps deleted long ago.
	OrphanedKeys int `json:"orphanedKeys"`
}

type vaultPending struct {
	Kind   string `json:"kind"` // signing | play
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
}

type vaultMoveFailure struct {
	Kind   string `json:"kind"` // signing | play
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type vaultMoveResult struct {
	Moved  []string           `json:"moved"`
	Failed []vaultMoveFailure `json:"failed"`
}

func (s *Server) vaultInfo(ctx context.Context, force bool) vaultInfo {
	info := vaultInfo{Mode: "off", Status: vaultStatus{State: "off", Detail: "1Password is not configured: keys are sealed in the database with RELEASE_HUB_SECRET_KEY."}}
	count := func(q string) (local, inVault int) {
		_ = s.DB.QueryRowContext(ctx, q).Scan(&local, &inVault)
		return
	}
	info.Local.SigningKeys, info.InVault.SigningKeys = count(`SELECT
		COALESCE(SUM(p.sign_keystore != '' AND p.sign_keystore NOT LIKE 'op:%'), 0),
		COALESCE(SUM(p.sign_keystore LIKE 'op:%'), 0)
		FROM app_platforms p JOIN apps a ON a.id = p.app_id`)
	// Platforms whose app is gone: apps deleted before foreign keys were on
	// for every connection (2026-11-15) left them behind. Their keys belong
	// to no app, so they are neither counted nor moved.
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_platforms
		WHERE sign_keystore != '' AND app_id NOT IN (SELECT id FROM apps)`).Scan(&info.OrphanedKeys)
	info.Local.PlayAccounts, info.InVault.PlayAccounts = count(`SELECT
		COALESCE(SUM(credentials NOT LIKE 'op:%'), 0),
		COALESCE(SUM(credentials LIKE 'op:%'), 0) FROM play_accounts`)
	info.Pending = s.pendingKeys(ctx)
	if s.vault == nil {
		return info
	}
	info.Configured, info.Mode = true, "external"
	if s.vault.bundled != nil {
		info.Mode = "bundled"
	}
	info.Status = s.vault.status(ctx, force)
	return info
}

func (s *Server) handleVaultStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.vaultInfo(r.Context(), r.URL.Query().Get("refresh") == "1"))
}

func (s *Server) handleVaultMigrate(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		writeErr(w, 400, "1Password is not configured: set OP_CONNECT_CREDENTIALS (or OP_CONNECT_HOST) and OP_CONNECT_TOKEN, then redeploy")
		return
	}
	if st := s.vault.status(r.Context(), true); st.State != "ready" {
		writeErr(w, 409, "1Password is not ready: "+st.Detail)
		return
	}
	if !s.moves.run.TryLock() {
		writeErr(w, 409, "a move is already running")
		return
	}
	defer s.moves.run.Unlock()
	res := s.migrateToVault(r.Context())
	s.moves.record(res)
	writeJSON(w, 200, res)
}

func (s *Server) migrateToVault(ctx context.Context) vaultMoveResult {
	res := vaultMoveResult{Moved: []string{}, Failed: []vaultMoveFailure{}}
	fail := func(kind, name string, err error) {
		res.Failed = append(res.Failed, vaultMoveFailure{kind, name, err.Error()})
		slog.Warn("key not moved to 1Password", "kind", kind, "name", name, "error", err)
	}

	type platRow struct {
		id                    int64
		slug, platform        string
		keystore, cfg, sha256 string
	}
	var plats []platRow
	rows, err := s.DB.QueryContext(ctx, `SELECT p.id, a.slug, p.platform, p.sign_keystore, p.sign_config, p.sign_sha256
		FROM app_platforms p JOIN apps a ON a.id = p.app_id
		WHERE p.sign_keystore != '' AND p.sign_keystore NOT LIKE 'op:%'
		ORDER BY a.slug, p.platform`)
	if err != nil {
		fail("signing", "*", err)
	} else {
		for rows.Next() {
			var p platRow
			if err := rows.Scan(&p.id, &p.slug, &p.platform, &p.keystore, &p.cfg, &p.sha256); err == nil {
				plats = append(plats, p)
			}
		}
		rows.Close()
	}
	for _, p := range plats {
		name := p.slug + "/" + p.platform
		if err := s.moveSigning(ctx, p.id, p.slug, p.platform, p.keystore, p.cfg, p.sha256); err != nil {
			fail("signing", name, err)
			continue
		}
		res.Moved = append(res.Moved, name+" signing key")
	}

	type acctRow struct {
		id    int64
		creds string
	}
	var accts []acctRow
	rows, err = s.DB.QueryContext(ctx, `SELECT id, credentials FROM play_accounts WHERE credentials NOT LIKE 'op:%' ORDER BY id`)
	if err != nil {
		fail("play", "*", err)
	} else {
		for rows.Next() {
			var a acctRow
			if err := rows.Scan(&a.id, &a.creds); err == nil {
				accts = append(accts, a)
			}
		}
		rows.Close()
	}
	for _, a := range accts {
		name, err := s.movePlayAccount(ctx, a.id, a.creds)
		if err != nil {
			fail("play", name, err)
			continue
		}
		res.Moved = append(res.Moved, "Play service account "+name)
	}
	return res
}

// pendingKeys names every key still sealed in the database. A reason comes
// from the last move, or from checks that need no network (the key opens
// with RELEASE_HUB_SECRET_KEY and matches its recorded sha256).
func (s *Server) pendingKeys(ctx context.Context) []vaultPending {
	out := []vaultPending{}
	rows, err := s.DB.QueryContext(ctx, `SELECT a.slug, p.platform, p.sign_keystore, p.sign_config, p.sign_sha256
		FROM app_platforms p JOIN apps a ON a.id = p.app_id
		WHERE p.sign_keystore != '' AND p.sign_keystore NOT LIKE 'op:%'
		ORDER BY a.slug, p.platform`)
	if err == nil {
		for rows.Next() {
			var slug, platform, ks, cfg, sha string
			if rows.Scan(&slug, &platform, &ks, &cfg, &sha) != nil {
				continue
			}
			p := vaultPending{Kind: "signing", Name: slug + "/" + platform}
			if _, _, err := openStoredSigning(ks, cfg, sha); err != nil {
				p.Reason = err.Error()
			} else if f, ok := s.moves.lastFailure(p.Kind, p.Name); ok {
				p.Reason = f.Reason
			}
			out = append(out, p)
		}
		rows.Close()
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT id, credentials FROM play_accounts WHERE credentials NOT LIKE 'op:%' ORDER BY id`)
	if err == nil {
		for rows.Next() {
			var id int64
			var enc string
			if rows.Scan(&id, &enc) != nil {
				continue
			}
			p := vaultPending{Kind: "play", Name: fmt.Sprintf("#%d", id)}
			if raw, err := decryptCreds(enc); err != nil {
				p.Reason = "open the stored key: " + err.Error()
			} else {
				var probe struct {
					ClientEmail string `json:"client_email"`
				}
				if json.Unmarshal(raw, &probe) == nil && probe.ClientEmail != "" {
					p.Name = probe.ClientEmail
				}
				if f, ok := s.moves.lastFailure(p.Kind, p.Name); ok {
					p.Reason = f.Reason
				}
			}
			out = append(out, p)
		}
		rows.Close()
	}
	return out
}

// openStoredSigning opens a sealed keystore and its config and checks the
// keystore against its recorded sha256.
func openStoredSigning(encKS, encCfg, wantSHA string) ([]byte, []byte, error) {
	ks, err := decryptCreds(encKS)
	if err != nil {
		return nil, nil, fmt.Errorf("open the stored keystore: %w", err)
	}
	cfgRaw, err := decryptCreds(encCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("open the stored signing config: %w", err)
	}
	sum := sha256.Sum256(ks)
	if wantSHA != "" && !strings.EqualFold(hex.EncodeToString(sum[:]), wantSHA) {
		return nil, nil, fmt.Errorf("the stored keystore doesn't match its recorded sha256; left as it is")
	}
	return ks, cfgRaw, nil
}

func (s *Server) moveSigning(ctx context.Context, id int64, slug, platform, encKS, encCfg, wantSHA string) error {
	ks, cfgRaw, err := openStoredSigning(encKS, encCfg, wantSHA)
	if err != nil {
		return err
	}
	var cfg signingConfig
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		return fmt.Errorf("the stored signing config is not readable: %w", err)
	}
	refs, err := s.vault.createItem(ctx, signingTitle(slug, platform), signingNotes(slug, platform), signingValues(ks, cfgRaw, cfg))
	if err != nil {
		return err
	}
	out, err := s.DB.ExecContext(ctx, `UPDATE app_platforms SET sign_keystore = ?, sign_config = ?
		WHERE id = ? AND sign_keystore = ? AND sign_config = ?`, refs[0], refs[1], id, encKS, encCfg)
	if n, _ := rowsAffected(out, err); n != 1 {
		s.vault.retire(ctx, refs[0])
		if err != nil {
			return err
		}
		return fmt.Errorf("the key changed while it was being moved; run the move again")
	}
	return nil
}

func (s *Server) movePlayAccount(ctx context.Context, id int64, enc string) (string, error) {
	name := fmt.Sprintf("#%d", id)
	raw, err := decryptCreds(enc)
	if err != nil {
		return name, fmt.Errorf("open the stored key: %w", err)
	}
	var probe struct {
		ClientEmail string `json:"client_email"`
	}
	if json.Unmarshal(raw, &probe) == nil && probe.ClientEmail != "" {
		name = probe.ClientEmail
	}
	ref, err := s.vault.createItem(ctx, playAccountTitle(name), playNotes, playValues(raw))
	if err != nil {
		return name, err
	}
	out, err := s.DB.ExecContext(ctx, `UPDATE play_accounts SET credentials = ? WHERE id = ? AND credentials = ?`, ref[0], id, enc)
	if n, _ := rowsAffected(out, err); n != 1 {
		s.vault.retire(ctx, ref[0])
		if err != nil {
			return name, err
		}
		return name, fmt.Errorf("the account changed while it was being moved; run the move again")
	}
	return name, nil
}

type sqlResult interface{ RowsAffected() (int64, error) }

func rowsAffected(res sqlResult, err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
