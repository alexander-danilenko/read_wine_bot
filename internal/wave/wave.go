// Package wave answers every private message with a waving hand.
package wave

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Handler returns a bot handler that replies 👋 to each message a user sends the bot in a
// private chat. It ignores group and channel messages so the bot stays quiet in the chat.
func Handler(log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil || msg.Chat.Type != models.ChatTypePrivate {
			return
		}

		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "👋"})
		if err != nil {
			log.ErrorContext(ctx, "send wave", "chat_id", msg.Chat.ID, "error", err)
		}
	}
}
