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
)

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
		COALESCE(SUM(sign_keystore != '' AND sign_keystore NOT LIKE 'op:%'), 0),
		COALESCE(SUM(sign_keystore LIKE 'op:%'), 0) FROM app_platforms`)
	info.Local.PlayAccounts, info.InVault.PlayAccounts = count(`SELECT
		COALESCE(SUM(credentials NOT LIKE 'op:%'), 0),
		COALESCE(SUM(credentials LIKE 'op:%'), 0) FROM play_accounts`)
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
	res := s.migrateToVault(r.Context())
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

func (s *Server) moveSigning(ctx context.Context, id int64, slug, platform, encKS, encCfg, wantSHA string) error {
	ks, err := decryptCreds(encKS)
	if err != nil {
		return fmt.Errorf("open the stored keystore: %w", err)
	}
	cfgRaw, err := decryptCreds(encCfg)
	if err != nil {
		return fmt.Errorf("open the stored signing config: %w", err)
	}
	sum := sha256.Sum256(ks)
	if wantSHA != "" && !strings.EqualFold(hex.EncodeToString(sum[:]), wantSHA) {
		return fmt.Errorf("the stored keystore doesn't match its recorded sha256; left as it is")
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
