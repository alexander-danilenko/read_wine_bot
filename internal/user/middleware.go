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
	Record(ctx context.Context, p Profile) error
	SetPendingCommand(ctx context.Context, telegramID int64, command string) error
}

// Middleware returns a bot middleware that runs before every handler. It records the profile
// of whoever sent the update, in any chat, so the users table always has current profiles and
// their revisions. A private command, or a text that isLabel reports as a menu button, clears
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

// record saves the sender's profile and, for a private command or menu button, clears the
// pending command.
func record(ctx context.Context, store Recorder, isLabel func(string) bool, log *slog.Logger, update *models.Update) {
	if from := sender(update); from != nil {
		recordSender(ctx, store, log, from)
	}

	msg := update.Message
	if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
		return
	}

	if !strings.HasPrefix(msg.Text, "/") && !isLabel(msg.Text) {
		return
	}

	if err := store.SetPendingCommand(ctx, msg.From.ID, ""); err != nil {
		log.ErrorContext(ctx, "save pending command", "user_id", msg.From.ID, "error", err)
	}
}

// sender returns the user behind the update, or nil when it has none. Inline queries, poll
// answers and payments never reach this bot, and chat_member updates are not recorded.
func sender(update *models.Update) *models.User {
	switch {
	case update.Message != nil:
		return messageSender(update.Message)
	case update.EditedMessage != nil:
		return messageSender(update.EditedMessage)
	case update.CallbackQuery != nil:
		return &update.CallbackQuery.From
	case update.MyChatMember != nil:
		return &update.MyChatMember.From
	case update.ChatJoinRequest != nil:
		return &update.ChatJoinRequest.From
	default:
		return nil
	}
}

// messageSender returns the user who sent msg, or nil when it was sent on behalf of a chat:
// a post made as the channel, an anonymous group admin, or the channel's automatic forward
// into its discussion chat. Telegram fills From with a placeholder account for those.
func messageSender(msg *models.Message) *models.User {
	if msg.SenderChat != nil {
		return nil
	}

	return msg.From
}

// recordSender saves the profile of the user who sent the update.
func recordSender(ctx context.Context, store Recorder, log *slog.Logger, from *models.User) {
	p := Profile{
		TelegramID:   from.ID,
		IsBot:        from.IsBot,
		FirstName:    from.FirstName,
		LastName:     from.LastName,
		Username:     from.Username,
		LanguageCode: from.LanguageCode,
		IsPremium:    from.IsPremium,
	}
	if err := store.Record(ctx, p); err != nil {
		log.ErrorContext(ctx, "record user", "user_id", from.ID, "error", err)
	}
}
