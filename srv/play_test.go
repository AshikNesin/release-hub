package srv

import "testing"

func TestTrackFor(t *testing.T) {
	cases := map[string]struct {
		track string
		ok    bool
	}{
		"public":    {"production", true},
		"internal":  {"internal", true},
		"api-share": {"", false}, // legacy alias
		"direct":     {"", false},
		"weird":     {"", false},
	}
	for ch, want := range cases {
		track, ok := trackFor(ch)
		if track != want.track || ok != want.ok {
			t.Errorf("trackFor(%q) = %q,%v want %q,%v", ch, track, ok, want.track, want.ok)
		}
	}
}

func TestEncryptDecryptCreds(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==") // 32 bytes b64
	enc, err := encryptCreds([]byte(`{"client_email":"a@b.c"}`))
	if err != nil {
		t.Fatal(err)
	}
	if enc == `{"client_email":"a@b.c"}` {
		t.Fatal("not encrypted")
	}
	dec, err := decryptCreds(enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(dec) != `{"client_email":"a@b.c"}` {
		t.Fatalf("roundtrip mismatch: %s", dec)
	}
}

func TestEncryptRequiresKey(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "")
	t.Setenv("ALLOW_PLAINTEXT_CREDS", "")
	if _, err := encryptCreds([]byte("x")); err == nil {
		t.Fatal("expected error without secret key")
	}
}
