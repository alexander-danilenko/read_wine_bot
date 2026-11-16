package i18n_test

import (
	"strings"
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

	for _, key := range []i18n.Key{i18n.Greeting, i18n.AppOpen, i18n.ReviewReady} {
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
		{name: "exact language", lang: "uk", want: "🍷 Відкрити застосунок"},
		{name: "region suffix dropped", lang: "en-US", want: "🍷 Open the app"},
		{name: "unknown language falls back", lang: "de", want: "🍷 Відкрити застосунок"},
		{name: "empty language falls back", lang: "", want: "🍷 Відкрити застосунок"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.Text(tt.lang, i18n.AppOpen); got != tt.want {
				t.Errorf("Text(%q) = %q, want %q", tt.lang, got, tt.want)
			}
		})
	}
}

func TestReviewReadyPlaceholder(t *testing.T) {
	c, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	for _, lang := range []string{"en", "uk"} {
		if got := c.Text(
			lang,
			i18n.ReviewReady,
			"https://t.me/c/1/2",
		); !strings.Contains(
			got,
			`href="https://t.me/c/1/2"`,
		) {
			t.Errorf("%s review.ready %q lacks the post link", lang, got)
		}
	}
}
