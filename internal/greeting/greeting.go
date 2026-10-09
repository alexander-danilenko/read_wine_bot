// Package greeting greets a user who sends /start, which Telegram sends when a user first opens the bot.
package greeting

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
)

// Command is the bot command, without the slash, that shows the greeting.
const Command = "start"

// Handler returns a handler that answers /start in a private chat with the greeting, in the
// language the user chose with /language or else the language of their Telegram app. It
// ignores the command in groups and channels.
func Handler(texts *i18n.Catalog, langs language.Reader, log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil || msg.Chat.Type != models.ChatTypePrivate {
			return
		}

		lang := language.UserLanguage(ctx, langs, log, msg.From)

		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: texts.Text(lang, i18n.Greeting)})
		if err != nil {
			log.ErrorContext(ctx, "send greeting", "chat_id", msg.Chat.ID, "error", err)
		}
	}
}
