// Package language tells which language the bot speaks to a user: the one they saved in the
// Mini App's settings, or else their Telegram app's.
package language

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot/models"
)

// Reader reads a user's chosen language. Language returns "" for a user who never chose.
type Reader interface {
	Language(ctx context.Context, telegramID int64) (string, error)
}

// UserLanguage returns the language the user chose in the settings, or else their Telegram app
// language; i18n.Catalog.Text falls back to Ukrainian after that. A read error is logged and
// falls back to the app language, so a database problem never silences a reply.
func UserLanguage(ctx context.Context, store Reader, log *slog.Logger, from *models.User) string {
	if from == nil {
		return ""
	}

	saved, err := store.Language(ctx, from.ID)
	if err != nil {
		log.ErrorContext(ctx, "read language", "user_id", from.ID, "error", err)

		return from.LanguageCode
	}

	if saved != "" {
		return saved
	}

	return from.LanguageCode
}
