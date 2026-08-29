package srv

import "testing"

func TestTrackFor(t *testing.T) {
	cases := map[string]struct {
		track string
		ok    bool
	}{
		"public":             {"production", true},
		"open":               {"beta", true},
		"internal":           {"internal", true},
		"direct":             {"", false},
		"api-share (legacy)": {"", false}, // rejected at upload layer; trackFor returns no track anyway
		"weird":              {"", false},
		// closed — the hub's simple one-track channel (auto-created)
		"closed": {"beta-testers", true},
		// closed:<name> — closed testing tracks by free-form name
		"closed:alpha":     {"alpha", true},
		"closed:team":      {"team", true},
		"closed:qa-2026":   {"qa-2026", true},
		"closed:a":         {"", false}, // too short (min 2)
		"closed:has space": {"", false}, // spaces not allowed
		"closed:":          {"", false}, // empty name
	}
	for ch, want := range cases {
		track, ok := trackFor(ch)
		if track != want.track || ok != want.ok {
			t.Errorf("trackFor(%q) = %q,%v want %q,%v", ch, track, ok, want.track, want.ok)
		}
	}
}

func TestTrackIsClosed(t *testing.T) {
	for ch, want := range map[string]bool{
		"internal": false, "open": false, "public": false, "direct": false,
		"closed": true, "closed:alpha": true, "closed:team": true,
		"closed:": false, "alpha": false,
	} {
		if got := trackIsClosed(ch); got != want {
			t.Errorf("trackIsClosed(%q) = %v want %v", ch, got, want)
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
