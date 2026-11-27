package i18n

import (
	"math"
	"strings"
	"testing"
)

// sampleArgs gives each key the args its callers pass, so a catalog with a
// missing or extra verb shows up as a "%!" in the output.
var sampleArgs = map[Key][]any{
	Logged: {"Rp25.000", "Food", "kopi", "2026-11-28"},
}

func TestCatalogsAreComplete(t *testing.T) {
	for _, locale := range []string{"id", "en"} {
		t.Run(locale, func(t *testing.T) {
			tr, err := New(locale)
			if err != nil {
				t.Fatal(err)
			}
			if len(tr.msgs) != int(numKeys) {
				t.Errorf("catalog has %d messages, want %d", len(tr.msgs), numKeys)
			}
			for k := range numKeys {
				msg, ok := tr.msgs[k]
				if !ok || strings.TrimSpace(msg) == "" {
					t.Errorf("key %d missing or empty", k)
					continue
				}
				if out := tr.T(k, sampleArgs[k]...); strings.Contains(out, "%!") {
					t.Errorf("key %d has mismatched format verbs: %q", k, out)
				}
			}
		})
	}
}

func TestNewUnknownLocale(t *testing.T) {
	if _, err := New("fr"); err == nil {
		t.Fatal("want error for unknown locale")
	}
}

func TestTranslate(t *testing.T) {
	tr, err := New("en")
	if err != nil {
		t.Fatal(err)
	}
	got := tr.T(Logged, "Rp25.000", "Food", "coffee", "2026-11-28")
	want := "✅ Rp25.000 · Food\n📝 coffee\n📅 2026-11-28"
	if got != want {
		t.Errorf("T(Logged) = %q, want %q", got, want)
	}
}

func TestRupiah(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "Rp0"},
		{5, "Rp5"},
		{999, "Rp999"},
		{1_000, "Rp1.000"},
		{25_000, "Rp25.000"},
		{1_250_000, "Rp1.250.000"},
		{1_000_000_000, "Rp1.000.000.000"},
		{-32_500, "-Rp32.500"},
		{math.MaxInt64, "Rp9.223.372.036.854.775.807"},
		{math.MinInt64, "-Rp9.223.372.036.854.775.808"},
	}
	for _, tt := range tests {
		if got := Rupiah(tt.in); got != tt.want {
			t.Errorf("Rupiah(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
