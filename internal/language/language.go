// Package language lets a user pick the bot's language with /language. The choice is not
// saved yet: the bot only confirms it.
package language

import (
	"context"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// Command is the bot command, without the slash, that shows the language buttons.
const Command = "language"

// CallbackPrefix starts the callback data of every language button; the language code follows.
const CallbackPrefix = "language:"

// option is one language button.
type option struct {
	code  string // locale file name in internal/i18n/locales, without .json
	label string // shown on the button in the language's own name, so every user finds it
}

// options returns the buttons in display order.
func options() []option {
	return []option{
		{code: "en", label: "🇺🇸 English"},
		{code: "uk", label: "🇺🇦 Українська"},
	}
}

// CommandHandler returns a handler that answers /language in a private chat with one button
// per language. It ignores the command in groups and channels.
func CommandHandler(texts *i18n.Catalog, log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil || msg.Chat.Type != models.ChatTypePrivate {
			return
		}

		var row []models.InlineKeyboardButton
		for _, o := range options() {
			row = append(row, models.InlineKeyboardButton{Text: o.label, CallbackData: CallbackPrefix + o.code})
		}

		var lang string
		if msg.From != nil {
			lang = msg.From.LanguageCode
		}

		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      msg.Chat.ID,
			Text:        texts.Text(lang, i18n.LanguagePrompt),
			ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{row}},
		})
		if err != nil {
			log.ErrorContext(ctx, "send language prompt", "chat_id", msg.Chat.ID, "error", err)
		}
	}
}

// ButtonHandler returns a handler for taps on the language buttons. It stops the button's
// loading spinner and confirms the choice in a new message. The choice is not saved.
func ButtonHandler(texts *i18n.Catalog, log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		query := update.CallbackQuery
		if query == nil {
			return
		}

		// Telegram shows a spinner on the button until the query is answered.
		if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID}); err != nil {
			log.ErrorContext(ctx, "answer language button", "user_id", query.From.ID, "error", err)
		}

		code := strings.TrimPrefix(query.Data, CallbackPrefix)
		for _, o := range options() {
			if o.code != code {
				continue
			}

			// The buttons are sent only in private chats, where the chat ID equals the user ID.
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: query.From.ID,
				Text:   texts.Text(query.From.LanguageCode, i18n.LanguageSelected, o.label),
			})
			if err != nil {
				log.ErrorContext(ctx, "send language selected", "user_id", query.From.ID, "error", err)
			}

			return
		}

		log.WarnContext(ctx, "unknown language button", "data", query.Data)
	}
}
