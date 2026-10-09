package user

import (
	"context"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Recorder saves what the middleware learns from each update.
type Recorder interface {
	SaveNames(ctx context.Context, telegramID int64, firstName, lastName string) error
	SetPendingCommand(ctx context.Context, telegramID int64, command string) error
}

// Middleware returns a bot middleware that runs before every handler. It saves the name of
// the sender of every private message and every button tap, so the users table always has
// current names. A private command, or a text that isLabel reports as a menu button, clears
// the pending command, so the user leaves any unfinished setup; the handler that runs next
// may start a new one. A store error is logged and the handler still runs, so a database
// problem never silences a reply.
func Middleware(store Recorder, isLabel func(text string) bool, log *slog.Logger) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			record(ctx, store, isLabel, log, update)
			next(ctx, b, update)
		}
	}
}

// record saves the sender's names and, for a private command or menu button, clears the
// pending command.
func record(ctx context.Context, store Recorder, isLabel func(string) bool, log *slog.Logger, update *models.Update) {
	if q := update.CallbackQuery; q != nil {
		saveNames(ctx, store, log, &q.From)

		return
	}

	msg := update.Message
	if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
		return
	}

	saveNames(ctx, store, log, msg.From)

	if !strings.HasPrefix(msg.Text, "/") && !isLabel(msg.Text) {
		return
	}

	if err := store.SetPendingCommand(ctx, msg.From.ID, ""); err != nil {
		log.ErrorContext(ctx, "save pending command", "user_id", msg.From.ID, "error", err)
	}
}

func saveNames(ctx context.Context, store Recorder, log *slog.Logger, from *models.User) {
	if err := store.SaveNames(ctx, from.ID, from.FirstName, from.LastName); err != nil {
		log.ErrorContext(ctx, "save user names", "user_id", from.ID, "error", err)
	}
}
