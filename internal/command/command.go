// Package command answers the bot's slash commands in a private chat and lists them in
// Telegram's command menu.
package command

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
)

// Commands, without the slash. Every command but Start is in the command menu and opens a
// screen of the Mini App, where giveaways are managed.
const (
	Start    = "start"    // shows the greeting and the button that opens the Mini App
	Help     = "help"     // opens Help: how the bot works and the channel rights it needs
	List     = "list"     // opens the giveaway list
	New      = "new"      // opens the new-giveaway form
	Language = "language" // opens Settings: the language and the user's channels
)

// entry is one command in the command menu and the Mini App screen it opens.
type entry struct {
	command     string
	description i18n.Key // starts with an emoji, so the eye finds the command
	query       string   // the screen, as a query added to the Mini App's URL; parseScreen in web/src/App.tsx reads it
	text        i18n.Key // the reply above the button
	button      i18n.Key // the button that opens the screen
}

// menu returns the command menu in display order, /help first, so a new user starts there.
func menu() []entry {
	return []entry{
		{Help, i18n.CommandHelp, "?help", i18n.ScreenHelp, i18n.ScreenHelpButton},
		{List, i18n.CommandList, "", i18n.ScreenList, i18n.ScreenListButton},
		{New, i18n.CommandNew, "?new", i18n.ScreenNew, i18n.ScreenNewButton},
		{Language, i18n.CommandLanguage, "?settings", i18n.ScreenSettings, i18n.ScreenSettingsButton},
	}
}

// Sender sends the one message with buttons in a user's private chat. *keyboard.Tracker
// implements it.
type Sender interface {
	SendHTML(ctx context.Context, b *bot.Bot, userID int64, text string, rows [][]models.InlineKeyboardButton)
}

// StartHandler returns a handler that answers /start with the greeting, the command menu as
// tappable commands, the bot's version, so the owner can see which build is deployed, and a
// button that opens the Mini App at appURL.
func StartHandler(
	texts *i18n.Catalog, langs language.Reader, kb Sender, appURL, version string, log *slog.Logger,
) bot.HandlerFunc {
	entries := menu()

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
			return
		}

		lang := language.UserLanguage(ctx, langs, log, msg.From)

		lines := make([]string, 0, len(entries))
		for _, e := range entries {
			lines = append(lines, "/"+e.command+" — "+texts.Text(lang, e.description))
		}

		text := texts.Text(lang, i18n.Greeting) + "\n\n" + strings.Join(lines, "\n") + "\n\nv" + version
		open := models.InlineKeyboardButton{
			Text:   texts.Text(lang, i18n.AppOpen),
			WebApp: &models.WebAppInfo{URL: appURL},
		}
		kb.SendHTML(ctx, b, msg.From.ID, text, [][]models.InlineKeyboardButton{{open}})
	}
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

// ScreenHandlers returns, for each command in the menu, a handler that answers it in a private
// chat with one line and a button that opens the command's screen of the Mini App at appURL.
// The texts are in the language the user chose, or else the language of their Telegram app.
func ScreenHandlers(
	texts *i18n.Catalog, langs language.Reader, kb Sender, appURL string, log *slog.Logger,
) map[string]bot.HandlerFunc {
	handlers := make(map[string]bot.HandlerFunc, len(menu()))
	for _, e := range menu() {
		handlers[e.command] = func(ctx context.Context, b *bot.Bot, update *models.Update) {
			msg := update.Message
			if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
				return
			}

			lang := language.UserLanguage(ctx, langs, log, msg.From)
			open := models.InlineKeyboardButton{
				Text:   texts.Text(lang, e.button),
				WebApp: &models.WebAppInfo{URL: appURL + e.query},
			}
			kb.SendHTML(ctx, b, msg.From.ID, texts.Text(lang, e.text), [][]models.InlineKeyboardButton{{open}})
		}
	}

	return handlers
}

// Register lists the commands in Telegram's command menu of private chats, in every language
// with a locale file, and in the default language for the rest.
func Register(ctx context.Context, b *bot.Bot, texts *i18n.Catalog) error {
	for _, lang := range append([]string{""}, texts.Langs()...) {
		commands := make([]models.BotCommand, 0, len(menu()))
		for _, e := range menu() {
			commands = append(
				commands,
				models.BotCommand{Command: e.command, Description: texts.Text(lang, e.description)},
			)
		}

		_, err := b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
			Commands:     commands,
			Scope:        &models.BotCommandScopeAllPrivateChats{},
			LanguageCode: lang,
		})
		if err != nil {
			return fmt.Errorf("set bot commands for language %q: %w", lang, err)
		}
	}

	return nil
}
