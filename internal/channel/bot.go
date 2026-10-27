package channel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// BotChannels saves the bot's status in a channel. *Store implements it.
type BotChannels interface {
	SetBotChannel(ctx context.Context, c BotChannel) error
}

// KnownChannels lists the channels the bot checks at startup and saves what it finds.
// *Store implements it.
type KnownChannels interface {
	BotChannels
	KnownChannels(ctx context.Context) ([]int64, error)
}

// BotHandler returns a handler for my_chat_member updates. When the bot is added to a
// channel, removed from it, or promoted or demoted there, it saves whether the bot is an
// admin now. Other chats are ignored. Errors are logged; Telegram expects no reply.
func BotHandler(channels BotChannels, log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, update *models.Update) {
		upd := update.MyChatMember
		if upd == nil || upd.Chat.Type != models.ChatTypeChannel {
			return
		}

		c := BotChannel{
			ChatID:     upd.Chat.ID,
			BotIsAdmin: upd.NewChatMember.Type == models.ChatMemberTypeAdministrator,
			ChangedAt:  time.Unix(int64(upd.Date), 0),
		}
		if err := channels.SetBotChannel(ctx, c); err != nil {
			log.ErrorContext(ctx, "record bot status in channel", "chat_id", c.ChatID, "error", err)
		}
	}
}

// CheckBotChannels asks Telegram whether the bot is still an admin of every known channel and
// saves the answers. It catches what changed while the bot was not running, and fills
// bot_channels for channels the bot joined before it was logging my_chat_member. A channel
// Telegram cannot answer about is logged and left as it was; a store error is returned.
func CheckBotChannels(ctx context.Context, b *bot.Bot, channels KnownChannels, now time.Time, log *slog.Logger) error {
	ids, err := channels.KnownChannels(ctx)
	if err != nil {
		return err //nolint:wrapcheck // the store's error says what it read
	}

	for _, id := range ids {
		isAdmin, err := botIsAdmin(ctx, b, id)
		if err != nil {
			log.WarnContext(ctx, "check bot status in channel", "chat_id", id, "error", err)

			continue
		}

		if err := channels.SetBotChannel(ctx, BotChannel{ChatID: id, BotIsAdmin: isAdmin, ChangedAt: now}); err != nil {
			return err //nolint:wrapcheck // the store's error names the chat
		}
	}

	return nil
}

// botIsAdmin reports whether the bot is an admin of channel chatID. Telegram answers 400 or
// 403 when the bot is no longer in the channel or the channel is gone, which means no.
func botIsAdmin(ctx context.Context, b *bot.Bot, chatID int64) (bool, error) {
	m, err := b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: chatID, UserID: b.ID()})
	if errors.Is(err, bot.ErrorForbidden) || errors.Is(err, bot.ErrorBadRequest) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("get bot member: %w", err)
	}

	return m.Type == models.ChatMemberTypeAdministrator, nil
}
