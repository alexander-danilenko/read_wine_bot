// Package command answers the bot's slash commands in a private chat and lists them in
// Telegram's command menu.
package command

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
)

// Commands, without the slash. Every command but Start is in the command menu.
const (
	Start        = "start"         // shows the greeting; Telegram sends it when a user first opens the bot
	New          = "new"           // starts a giveaway setup
	Edit         = "edit"          // starts editing a giveaway
	List         = "list"          // lists the user's running giveaways
	ListFinished = "list_finished" // lists the user's finished giveaways page by page
	Language     = "language"      // shows the language buttons
	Help         = "help"          // explains how the bot works and the channel rights it needs
	Cancel       = "cancel"        // leaves any unfinished step
)

// entry is one command in the command menu.
type entry struct {
	command     string
	description i18n.Key // starts with an emoji, so the eye finds the command
}

// menu returns the command menu in display order.
func menu() []entry {
	return []entry{
		{New, i18n.CommandNew},
		{Edit, i18n.CommandEdit},
		{List, i18n.CommandList},
		{ListFinished, i18n.CommandListFinished},
		{Language, i18n.CommandLanguage},
		{Help, i18n.CommandHelp},
		{Cancel, i18n.CommandCancel},
	}
}

// StartHandler returns a handler that answers /start with the greeting, the command menu as
// tappable commands with /help first, so a new user starts there, and the bot's version, so
// the owner can see which build is deployed.
func StartHandler(texts *i18n.Catalog, langs language.Reader, version string, log *slog.Logger) bot.HandlerFunc {
	entries := append(
		[]entry{{Help, i18n.CommandHelp}},
		slices.DeleteFunc(menu(), func(e entry) bool { return e.command == Help })...,
	)

	return reply(langs, log, func(lang string) string {
		lines := make([]string, 0, len(entries))
		for _, e := range entries {
			lines = append(lines, "/"+e.command+" — "+texts.Text(lang, e.description))
		}

		return texts.Text(lang, i18n.Greeting) + "\n\n" + strings.Join(lines, "\n") + "\n\nv" + version
	})
}

// HelpHandler returns a handler that answers /help: how a giveaway works and the channel
// rights the bot needs.
func HelpHandler(texts *i18n.Catalog, langs language.Reader, log *slog.Logger) bot.HandlerFunc {
	return reply(langs, log, func(lang string) string { return texts.Text(lang, i18n.Help) })
}

// CancelHandler returns a handler that answers /cancel. user.Middleware has already left the
// unfinished step and deleted its buttons, as it does for every command.
func CancelHandler(texts *i18n.Catalog, langs language.Reader, log *slog.Logger) bot.HandlerFunc {
	return reply(langs, log, func(lang string) string { return texts.Text(lang, i18n.CommandCancelled) })
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

// reply returns a handler that answers a command in a private chat with the text for the
// language the user chose, or else the language of their Telegram app. The reply also takes
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
			ChatID:      msg.Chat.ID,
			Text:        text(lang),
			ReplyMarkup: &models.ReplyKeyboardRemove{RemoveKeyboard: true},
		})
		if err != nil {
			log.ErrorContext(ctx, "reply to command", "chat_id", msg.Chat.ID, "command", msg.Text, "error", err)
		}
	}
}
