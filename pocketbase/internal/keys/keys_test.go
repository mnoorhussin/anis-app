package keys

import (
	"strings"
	"testing"
)

func TestNewWidgetKeyShape(t *testing.T) {
	k := NewWidgetKey()
	if !strings.HasPrefix(k, WidgetKeyPrefix) {
		t.Fatalf("missing prefix: %q", k)
	}
	if !IsWidgetKey(k) {
		t.Fatalf("generated key does not validate: %q", k)
	}
	// 16 bytes in base32 without padding.
	if want := len(WidgetKeyPrefix) + 26; len(k) != want {
		t.Errorf("length = %d, want %d (%q)", len(k), want, k)
	}
}

func TestNewWidgetKeyIsUnique(t *testing.T) {
	// Not a randomness test — a guard against someone replacing the CSPRNG
	// with a counter or a time-seeded source.
	seen := make(map[string]bool, 1000)
	for range 1000 {
		k := NewWidgetKey()
		if seen[k] {
			t.Fatalf("duplicate key generated: %q", k)
		}
		seen[k] = true
	}
}

func TestNewWidgetKeyAvoidsConfusableCharacters(t *testing.T) {
	// Keys get read aloud and retyped. I/L/O/U must never appear.
	for range 200 {
		body := NewWidgetKey()[len(WidgetKeyPrefix):]
		if i := strings.IndexAny(body, "ILOU"); i >= 0 {
			t.Fatalf("confusable character %q in key body %q", body[i], body)
		}
	}
}

func TestIsWidgetKeyRejectsJunk(t *testing.T) {
	valid := NewWidgetKey()
	cases := []struct {
		in   string
		want bool
		why  string
	}{
		{valid, true, "freshly generated"},
		{"", false, "empty"},
		{"wk_", false, "prefix only"},
		{strings.TrimPrefix(valid, WidgetKeyPrefix), false, "no prefix"},
		{"pk_" + strings.TrimPrefix(valid, WidgetKeyPrefix), false, "wrong prefix"},
		{valid + "A", false, "too long"},
		{valid[:len(valid)-1], false, "too short"},
		{WidgetKeyPrefix + strings.Repeat("I", 26), false, "excluded alphabet character"},
		{WidgetKeyPrefix + strings.Repeat("!", 26), false, "non-alphabet character"},
		// A SQL fragment must be rejected on shape alone, before any query.
		{"wk_' OR 1=1 --", false, "injection attempt"},
	}
	for _, c := range cases {
		if got := IsWidgetKey(c.in); got != c.want {
			t.Errorf("IsWidgetKey(%q) = %v, want %v (%s)", c.in, got, c.want, c.why)
		}
	}
}
