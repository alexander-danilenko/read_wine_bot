// Package command answers the bot's slash commands in a private chat and lists them in
// Telegram's command menu.
package command

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
	"github.com/alexander-danilenko/read_wine_bot/internal/menu"
)

// Commands, without the slash.
const (
	Start  = "start"  // shows the greeting; Telegram sends it when a user first opens the bot
	Cancel = "cancel" // leaves any unfinished step
)

// StartHandler returns a handler that answers /start with the greeting.
func StartHandler(texts *i18n.Catalog, langs language.Reader, log *slog.Logger) bot.HandlerFunc {
	return reply(texts, langs, log, i18n.Greeting)
}

// CancelHandler returns a handler that answers /cancel. user.Middleware has already left the
// unfinished step and deleted its buttons, as it does for every command.
func CancelHandler(texts *i18n.Catalog, langs language.Reader, log *slog.Logger) bot.HandlerFunc {
	return reply(texts, langs, log, i18n.CommandCancelled)
}

// Register lists the commands in Telegram's command menu of private chats, in every language
// with a locale file, and in the default language for the rest.
func Register(ctx context.Context, b *bot.Bot, texts *i18n.Catalog) error {
	for _, lang := range append([]string{""}, texts.Langs()...) {
		_, err := b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
			Commands: []models.BotCommand{
				{Command: Start, Description: texts.Text(lang, i18n.CommandStart)},
				{Command: Cancel, Description: texts.Text(lang, i18n.CommandCancel)},
			},
			Scope:        &models.BotCommandScopeAllPrivateChats{},
			LanguageCode: lang,
		})
		if err != nil {
			return fmt.Errorf("set bot commands for language %q: %w", lang, err)
		}
	}

	return nil
}

// reply returns a handler that answers a command in a private chat with text and the menu
// keyboard, in the language the user chose or else the language of their Telegram app. It
// ignores the command in groups and channels.
func reply(texts *i18n.Catalog, langs language.Reader, log *slog.Logger, text i18n.Key) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil || msg.Chat.Type != models.ChatTypePrivate {
			return
		}

		lang := language.UserLanguage(ctx, langs, log, msg.From)

		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      msg.Chat.ID,
			Text:        texts.Text(lang, text),
			ReplyMarkup: menu.Keyboard(texts, lang),
		})
		if err != nil {
			log.ErrorContext(ctx, "reply to command", "chat_id", msg.Chat.ID, "text", text, "error", err)
		}
	}
}
