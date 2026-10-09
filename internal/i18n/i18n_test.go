package i18n_test

import (
	"testing"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// Locale files are embedded at build time, so a passing Load here means Load cannot fail at
// runtime either.
func TestLoad(t *testing.T) {
	if _, err := i18n.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

// Catches a Key constant added in Go but not in the locale files: Text would return the bare key.
func TestEveryKeyTranslated(t *testing.T) {
	c, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []i18n.Key{i18n.Greeting, i18n.GiveawayWinner, i18n.SpamRemoved} {
		if got := c.Text(i18n.DefaultLang, key, "x"); got == string(key) {
			t.Errorf("key %q has no text in %s.json", key, i18n.DefaultLang)
		}
	}
}

func TestText(t *testing.T) {
	c, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		lang string
		want string
	}{
		{name: "exact language", lang: "ru", want: "Победитель розыгрыша: @alice 🎉"},
		{name: "region suffix dropped", lang: "en-US", want: "Giveaway winner: @alice 🎉"},
		{name: "unknown language falls back", lang: "de", want: "Giveaway winner: @alice 🎉"},
		{name: "empty language falls back", lang: "", want: "Giveaway winner: @alice 🎉"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.Text(tt.lang, i18n.GiveawayWinner, "@alice"); got != tt.want {
				t.Errorf("Text(%q) = %q, want %q", tt.lang, got, tt.want)
			}
		})
	}
}
