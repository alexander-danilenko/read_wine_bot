// Package command answers /start in a private chat and points the chat's menu button at the
// Mini App.
package command

import (
	"context"
	"fmt"
	"html"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
)

// Start is the one command, without the slash: Telegram sends it when a user opens the chat.
// Everything else happens in the Mini App.
const Start = "start"

// Sender sends the one message with buttons in a user's private chat. *keyboard.Tracker
// implements it.
type Sender interface {
	SendHTML(ctx context.Context, b *bot.Bot, userID int64, text string, rows [][]models.InlineKeyboardButton)
}

// Channel returns the title and username of the primary channel, read from Telegram on every
// /start, so a renamed channel shows its new name.
type Channel func(ctx context.Context, b *bot.Bot) (title, username string, err error)

// StartHandler returns a handler that answers /start with the greeting naming the primary
// channel, the bot's version, so the owner can see which build is deployed, and a button that
// opens the Mini App at appURL. When the channel cannot be read, the greeting leaves it out.
func StartHandler(
	texts *i18n.Catalog, langs language.Reader, kb Sender, channel Channel, appURL, version string, log *slog.Logger,
) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
			return
		}

		lang := language.UserLanguage(ctx, langs, log, msg.From)

		greeting := texts.Text(lang, i18n.GreetingPlain)
		if title, username, err := channel(ctx, b); err != nil {
			log.ErrorContext(ctx, "read primary channel for the greeting", "error", err)
		} else {
			greeting = texts.Text(lang, i18n.Greeting, channelHTML(title, username))
		}

		text := greeting + "\n\nv" + version
		open := models.InlineKeyboardButton{
			Text:   texts.Text(lang, i18n.AppOpen),
			WebApp: &models.WebAppInfo{URL: appURL},
		}
		kb.SendHTML(ctx, b, msg.From.ID, text, [][]models.InlineKeyboardButton{{open}})
	}
}

// channelHTML is the channel's title, linked to the channel when it has a public username.
func channelHTML(title, username string) string {
	if username == "" {
		return "<b>" + html.EscapeString(title) + "</b>"
	}

	return `<a href="https://t.me/` + username + `">` + html.EscapeString(title) + "</a>"
}

// SetMenuButton makes the chat's menu button open the Mini App at appURL, labelled in the
// default language: Telegram has one menu button for every language.
func SetMenuButton(ctx context.Context, b *bot.Bot, texts *i18n.Catalog, appURL string) error {
	_, err := b.SetChatMenuButton(ctx, &bot.SetChatMenuButtonParams{
		MenuButton: models.MenuButtonWebApp{
			Type:   models.MenuButtonTypeWebApp,
			Text:   texts.Text(i18n.DefaultLang, i18n.AppMenu),
			WebApp: models.WebAppInfo{URL: appURL},
		},
	})
	if err != nil {
		return fmt.Errorf("set menu button: %w", err)
	}

	return nil
}

// ClearCommands empties Telegram's command menu of private chats, in every language with a
// locale file and in the default one, so commands an older version registered disappear.
func ClearCommands(ctx context.Context, b *bot.Bot, texts *i18n.Catalog) error {
	for _, lang := range append([]string{""}, texts.Langs()...) {
		_, err := b.DeleteMyCommands(ctx, &bot.DeleteMyCommandsParams{
			Scope:        &models.BotCommandScopeAllPrivateChats{},
			LanguageCode: lang,
		})
		if err != nil {
			return fmt.Errorf("clear bot commands for language %q: %w", lang, err)
		}
	}

	return nil
}
