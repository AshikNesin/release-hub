package srv

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
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
