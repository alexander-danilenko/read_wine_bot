package user

import (
	"context"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// giveawayCommand is the only command that leaves the bot waiting for the user's next message.
const giveawayCommand = "giveaway"

// Recorder saves what the middleware learns from each update.
type Recorder interface {
	SaveNames(ctx context.Context, telegramID int64, firstName, lastName string) error
	SetPendingCommand(ctx context.Context, telegramID int64, command string) error
}

// Middleware returns a bot middleware that runs before every handler. It saves the name of
// the sender of every private message and every button tap, so the users table always has
// current names. A private command also sets the pending command: /giveaway makes the bot
// wait for a forwarded post, and any other command cancels that wait. A store error is
// logged and the handler still runs, so a database problem never silences a reply.
func Middleware(store Recorder, log *slog.Logger) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			record(ctx, store, log, update)
			next(ctx, b, update)
		}
	}
}

// record saves the sender's names and, for a private command, the pending command.
func record(ctx context.Context, store Recorder, log *slog.Logger, update *models.Update) {
	if q := update.CallbackQuery; q != nil {
		saveNames(ctx, store, log, &q.From)

		return
	}

	msg := update.Message
	if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
		return
	}

	saveNames(ctx, store, log, msg.From)

	command, ok := commandName(msg.Text)
	if !ok {
		return
	}

	pending := ""
	if command == giveawayCommand {
		pending = giveawayCommand
	}

	if err := store.SetPendingCommand(ctx, msg.From.ID, pending); err != nil {
		log.ErrorContext(ctx, "save pending command", "user_id", msg.From.ID, "error", err)
	}
}

func saveNames(ctx context.Context, store Recorder, log *slog.Logger, from *models.User) {
	if err := store.SaveNames(ctx, from.ID, from.FirstName, from.LastName); err != nil {
		log.ErrorContext(ctx, "save user names", "user_id", from.ID, "error", err)
	}
}

// commandName returns "giveaway" for "/giveaway@read_wine_bot now", and false for text that
// is not a command.
func commandName(text string) (string, bool) {
	word, _, _ := strings.Cut(text, " ")
	name, ok := strings.CutPrefix(word, "/")
	if !ok || name == "" {
		return "", false
	}

	name, _, _ = strings.Cut(name, "@")

	return name, true
}
