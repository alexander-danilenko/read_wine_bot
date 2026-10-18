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

// Clearer deletes the message with buttons in a user's private chat. *keyboard.Tracker
// implements it.
type Clearer interface {
	Clear(ctx context.Context, b *bot.Bot, userID int64)
}

// Middleware returns a bot middleware that runs before every handler. It records the profile
// of whoever sent the update, in any chat, so the users table always has current profiles and
// their revisions. A private command clears the pending command and deletes the message with
// buttons, so the user leaves any unfinished setup; the handler that runs next may start a
// new one. A store error is logged and the
// handler still runs, so a database problem never silences a reply.
func Middleware(store Recorder, kb Clearer, log *slog.Logger) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if from := sender(update); from != nil {
				recordSender(ctx, store, log, from)
			}

			if msg := update.Message; leaves(msg) {
				if err := store.SetPendingCommand(ctx, msg.From.ID, ""); err != nil {
					log.ErrorContext(ctx, "save pending command", "user_id", msg.From.ID, "error", err)
				}

				kb.Clear(ctx, b, msg.From.ID)
			}

			next(ctx, b, update)
		}
	}
}

// leaves reports whether msg is a private command, which leaves any unfinished setup.
func leaves(msg *models.Message) bool {
	return msg != nil && msg.From != nil && msg.Chat.Type == models.ChatTypePrivate && strings.HasPrefix(msg.Text, "/")
}

// sender returns the user behind the update, or nil when it has none. Inline queries, poll
// answers and payments never reach this bot. A chat_member update's subscriber is recorded by
// internal/channel instead, because From there is the admin when one removed the subscriber.
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

// ProfileOf returns the profile Telegram sent about u.
func ProfileOf(u *models.User) Profile {
	return Profile{
		TelegramID:   u.ID,
		IsBot:        u.IsBot,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		Username:     u.Username,
		LanguageCode: u.LanguageCode,
		IsPremium:    u.IsPremium,
	}
}

// recordSender saves the profile of the user who sent the update.
func recordSender(ctx context.Context, store Recorder, log *slog.Logger, from *models.User) {
	if err := store.Record(ctx, ProfileOf(from)); err != nil {
		log.ErrorContext(ctx, "record user", "user_id", from.ID, "error", err)
	}
}
