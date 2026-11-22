package srv

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Credential encryption at rest: AES-256-GCM with a key from the
// RELEASE_HUB_SECRET_KEY env var (32 raw bytes, base64- or hex-encoded,
// or any string < 32 bytes which is hashed; recommend the encoded forms).
// Without a key configured, credentials are stored base64-obfuscated —
// acceptable only for dev; the API rejects enable without a key unless
// ALLOW_PLAINTEXT_CREDS=1.
func loadSecretKey() ([]byte, error) {
	k := envStr("RELEASE_HUB_SECRET_KEY")
	if k == "" {
		return nil, nil
	}
	if b, err := base64.StdEncoding.DecodeString(k); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := hexDecode(k); err == nil && len(b) == 32 {
		return b, nil
	}
	// hash arbitrary passphrase to 32 bytes
	out := make([]byte, 32)
	copy(out, []byte(k))
	// not a KDF, but key material isn't attacker-controlled here
	return sha256Sum(k), nil
}

func encryptCreds(plain []byte) (string, error) {
	key, err := loadSecretKey()
	if err != nil {
		return "", err
	}
	if key == nil {
		if envStr("ALLOW_PLAINTEXT_CREDS") != "1" {
			return "", errors.New("set RELEASE_HUB_SECRET_KEY (32-byte base64/hex) to store Play credentials")
		}
		return "plain:" + base64.StdEncoding.EncodeToString(plain), nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return "aes:" + base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, plain, nil)), nil
}

func decryptCreds(stored string) ([]byte, error) {
	switch {
	case strings.HasPrefix(stored, "aes:"):
		key, err := loadSecretKey()
		if err != nil || key == nil {
			return nil, errors.New("credentials are encrypted but RELEASE_HUB_SECRET_KEY is not set")
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, "aes:"))
		if err != nil {
			return nil, err
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		if len(raw) < gcm.NonceSize() {
			return nil, errors.New("ciphertext too short")
		}
		return gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	case strings.HasPrefix(stored, "plain:"):
		return base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, "plain:"))
	default:
		return nil, errors.New("unknown credential format")
	}
}

func envStr(k string) string { return os.Getenv(k) }

func sha256Sum(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }

func hexDecode(s string) ([]byte, error) {
	if len(s) != 64 {
		return nil, fmt.Errorf("hex key must be 64 chars, got %d", len(s))
	}
	return hex.DecodeString(s)
}

// ---- stored secrets: sealed in the database or kept in 1Password ----

// displaySecretTimeout bounds reads made only to show something (a Play
// account's email, a key alias): a page never waits on a cold Connect.
const displaySecretTimeout = 2 * time.Second

// openSecret returns a stored secret's bytes, whichever way it is stored:
// a 1Password reference (vault.go) or a sealed value.
func (s *Server) openSecret(ctx context.Context, stored string) ([]byte, error) {
	if strings.HasPrefix(stored, opRefPrefix) {
		if s.vault == nil {
			return nil, errors.New("this key is stored in 1Password, but 1Password Connect is not configured (set OP_CONNECT_CREDENTIALS or OP_CONNECT_HOST, and OP_CONNECT_TOKEN)")
		}
		return s.vault.resolve(ctx, stored)
	}
	return decryptCreds(stored)
}

// sealSecrets stores values together: as one 1Password item when Connect
// is configured, otherwise each sealed with RELEASE_HUB_SECRET_KEY. It
// returns what to store per value, in order.
func (s *Server) sealSecrets(ctx context.Context, title, notes string, values []vaultValue) ([]string, error) {
	if s.vault != nil {
		return s.vault.createItem(ctx, title, notes, values)
	}
	out := make([]string, len(values))
	for i, v := range values {
		enc, err := encryptCreds(v.Data)
		if err != nil {
			return nil, err
		}
		out[i] = enc
	}
	return out, nil
}

// retireSecrets is called when release-hub stops using stored values (a
// key replaced or removed). Sealed values simply go with their row; a
// 1Password item is kept, tagged release-hub/retired, never deleted.
func (s *Server) retireSecrets(ctx context.Context, stored ...string) {
	if s.vault == nil {
		return
	}
	for _, v := range stored {
		if strings.HasPrefix(v, opRefPrefix) {
			s.vault.retire(ctx, v)
		}
	}
}

func signingTitle(slug, platform string) string {
	return fmt.Sprintf("release-hub: %s (%s) signing key", slug, platform)
}

// sealSigning stores a keystore and its config. In 1Password it is one item
// whose readable fields (alias, passwords, sha256) match what CI fetches.
func (s *Server) sealSigning(ctx context.Context, slug, platform string, ks []byte, cfg signingConfig) (encKS, encCfg string, err error) {
	cfgJSON, _ := json.Marshal(cfg)
	refs, err := s.sealSecrets(ctx, signingTitle(slug, platform), signingNotes(slug, platform), signingValues(ks, cfgJSON, cfg))
	if err != nil {
		return "", "", err
	}
	return refs[0], refs[1], nil
}

func signingNotes(slug, platform string) string {
	return "Signing keystore of " + slug + " (" + platform + "), kept by release-hub. Back it up: without it, installed apps can't be updated. The keystore_base64 field is the keystore file, base64 encoded."
}

// signingValues: the keystore and config first (what the row references),
// then readable copies for whoever opens the item in 1Password.
func signingValues(ks, cfgJSON []byte, cfg signingConfig) []vaultValue {
	sum := sha256.Sum256(ks)
	return []vaultValue{
		{ID: "keystore", Label: "keystore (base64)", Data: ks, Binary: true},
		{ID: "signing_config", Label: "signing config (JSON)", Data: cfgJSON, Concealed: true},
		{ID: "key_alias", Label: "key alias", Data: []byte(cfg.KeyAlias)},
		{ID: "store_password", Label: "store password", Data: []byte(cfg.StorePassword), Concealed: true},
		{ID: "key_password", Label: "key password", Data: []byte(cfg.KeyPassword), Concealed: true},
		{ID: "keystore_sha256", Label: "keystore sha256", Data: []byte(hex.EncodeToString(sum[:]))},
	}
}

func playAccountTitle(email string) string {
	return "release-hub: Google Play service account " + email
}

const playNotes = "Google Play service-account JSON key release-hub publishes with."

func playValues(raw []byte) []vaultValue {
	return []vaultValue{{ID: "service_account_json", Label: "service account JSON", Data: raw, Concealed: true}}
}

func (s *Server) sealPlayAccount(ctx context.Context, email string, raw []byte) (string, error) {
	refs, err := s.sealSecrets(ctx, playAccountTitle(email), playNotes, playValues(raw))
	if err != nil {
		return "", err
	}
	return refs[0], nil
}
