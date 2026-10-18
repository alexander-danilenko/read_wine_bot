// Package language lets a user pick the bot's language from the menu and saves the choice.
package language

import (
	"context"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/keyboard"
)

// CallbackPrefix starts the callback data of every language button; the language code follows.
const CallbackPrefix = "language:"

// option is one language button.
type option struct {
	code  string // locale file name in internal/i18n/locales, without .json
	label string // shown on the button in the language's own name, so every user finds it
}

// Reader reads a user's chosen language. Language returns "" for a user who never chose.
type Reader interface {
	Language(ctx context.Context, telegramID int64) (string, error)
}

// Store reads and saves a user's chosen language.
type Store interface {
	Reader
	SetLanguage(ctx context.Context, telegramID int64, lang string) error
}

// options returns the buttons in display order.
func options() []option {
	return []option{
		{code: "en", label: "🇺🇸 English"},
		{code: "uk", label: "🇺🇦 Українська"},
	}
}

// PromptHandler returns a handler that answers /language in a private chat
// with one button per language, in the user's saved language. It ignores groups and channels.
func PromptHandler(texts *i18n.Catalog, store Store, kb *keyboard.Tracker, log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil || msg.Chat.Type != models.ChatTypePrivate {
			return
		}

		var row []models.InlineKeyboardButton
		for _, o := range options() {
			row = append(row, models.InlineKeyboardButton{Text: o.label, CallbackData: CallbackPrefix + o.code})
		}

		text := texts.Text(UserLanguage(ctx, store, log, msg.From), i18n.LanguagePrompt)
		kb.Send(ctx, b, msg.Chat.ID, text, [][]models.InlineKeyboardButton{row})
	}
}

// ButtonHandler returns a handler for taps on the language buttons. It stops the button's
// loading spinner, deletes the language buttons, saves the choice and confirms it in the
// chosen language. If saving fails, it logs the error and still confirms, so the user is not
// left without an answer.
func ButtonHandler(texts *i18n.Catalog, store Store, kb *keyboard.Tracker, log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		query := update.CallbackQuery
		if query == nil {
			return
		}

		// Telegram shows a spinner on the button until the query is answered.
		if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID}); err != nil {
			log.ErrorContext(ctx, "answer language button", "user_id", query.From.ID, "error", err)
		}

		kb.Discard(ctx, b, query.Message)

		code := strings.TrimPrefix(query.Data, CallbackPrefix)
		for _, o := range options() {
			if o.code != code {
				continue
			}

			if err := store.SetLanguage(ctx, query.From.ID, o.code); err != nil {
				log.ErrorContext(ctx, "save language", "user_id", query.From.ID, "error", err)
			}

			// The buttons are sent only in private chats, where the chat ID equals the user ID.
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: query.From.ID,
				Text:   texts.Text(o.code, i18n.LanguageSelected, o.label),
			})
			if err != nil {
				log.ErrorContext(ctx, "send language selected", "user_id", query.From.ID, "error", err)
			}

			return
		}

		log.WarnContext(ctx, "unknown language button", "data", query.Data)
	}
}

// UserLanguage returns the language the user chose from the menu, or else their Telegram app
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
