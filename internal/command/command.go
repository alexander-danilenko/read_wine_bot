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

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
)

// Commands, without the slash. Every command but Start is in the command menu. Giveaways are
// managed in the Mini App, which the menu button opens.
const (
	Start    = "start"    // shows the greeting and the button that opens the Mini App
	Language = "language" // shows the language buttons
	Help     = "help"     // explains how the bot works and the channel rights it needs
)

// entry is one command in the command menu.
type entry struct {
	command     string
	description i18n.Key // starts with an emoji, so the eye finds the command
}

// menu returns the command menu in display order, /help first, so a new user starts there.
func menu() []entry {
	return []entry{
		{Help, i18n.CommandHelp},
		{Language, i18n.CommandLanguage},
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

// HelpHandler returns a handler that answers /help: how a giveaway works and the channel
// rights the bot needs, with links that add the bot to a channel with those rights or, for a
// partner channel, with none. Only getMe tells the bot's username, so the handler asks for it
// on every /help.
func HelpHandler(texts *i18n.Catalog, langs language.Reader, log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		me, err := b.GetMe(ctx)
		if err != nil {
			log.ErrorContext(ctx, "get bot username for /help", "error", err)

			return
		}

		link, partner := giveaway.AddToChannelLink(me.Username), giveaway.AddToPartnerChannelLink(me.Username)
		reply(
			langs,
			log,
			func(lang string) string { return texts.Text(lang, i18n.Help, link, partner) },
		)(
			ctx,
			b,
			update,
		)
	}
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

// reply returns a handler that answers a command in a private chat with the HTML text for
// the language the user chose, or else the language of their Telegram app. The reply also takes
// away the menu keyboard that older versions of the bot sent. It ignores the command in
// groups and channels.
func reply(langs language.Reader, log *slog.Logger, text func(lang string) string) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil || msg.Chat.Type != models.ChatTypePrivate {
			return
		}

		lang := language.UserLanguage(ctx, langs, log, msg.From)

		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:             msg.Chat.ID,
			Text:               text(lang),
			ParseMode:          models.ParseModeHTML,
			LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
			ReplyMarkup:        &models.ReplyKeyboardRemove{RemoveKeyboard: true},
		})
		if err != nil {
			log.ErrorContext(ctx, "reply to command", "chat_id", msg.Chat.ID, "command", msg.Text, "error", err)
		}
	}
}
