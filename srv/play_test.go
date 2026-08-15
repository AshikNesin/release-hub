package srv

import "testing"

func TestTrackFor(t *testing.T) {
	cases := map[string]struct {
		track string
		ok    bool
	}{
		"public":    {"production", true},
		"internal":  {"internal", true},
		"api-share": {"", false},
		"weird":     {"", false},
	}
	for ch, want := range cases {
		track, ok := trackFor(ch)
		if track != want.track || ok != want.ok {
			t.Errorf("trackFor(%q) = %q,%v want %q,%v", ch, track, ok, want.track, want.ok)
		}
	}
}
