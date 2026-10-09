package language_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/language"
)

// fakeReader returns a fixed language and error.
type fakeReader struct {
	lang string
	err  error
}

func (f fakeReader) Language(context.Context, int64) (string, error) { return f.lang, f.err }

func TestUserLanguage(t *testing.T) {
	app := &models.User{ID: 1, LanguageCode: "en-US"}

	tests := []struct {
		name   string
		reader fakeReader
		from   *models.User
		want   string
	}{
		{name: "saved choice wins", reader: fakeReader{lang: "uk"}, from: app, want: "uk"},
		{name: "no choice uses app language", from: app, want: "en-US"},
		{
			name:   "read error uses app language",
			reader: fakeReader{err: errors.New("db down")},
			from:   app,
			want:   "en-US",
		},
		{name: "no sender", reader: fakeReader{lang: "uk"}, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := language.UserLanguage(t.Context(), tc.reader, slog.New(slog.DiscardHandler), tc.from)
			if got != tc.want {
				t.Errorf("UserLanguage = %q, want %q", got, tc.want)
			}
		})
	}
}
