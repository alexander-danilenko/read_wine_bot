package channel

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// Events saves membership events. *Store implements it.
type Events interface {
	AddEvent(ctx context.Context, e Event) error
}

// Posts saves channel posts. *Store implements it.
type Posts interface {
	AddPost(ctx context.Context, p Post) error
}

// Profiles saves a user's profile. *user.Store implements it.
type Profiles interface {
	Record(ctx context.Context, p user.Profile) error
}

// MemberHandler returns a handler for chat_member updates. When a user subscribes to, leaves
// or is removed from a channel, it records the user's profile and then the event, which
// references the users row. Other chats and other status changes are ignored. Errors are
// logged; Telegram expects no reply.
func MemberHandler(events Events, profiles Profiles, log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, update *models.Update) {
		upd := update.ChatMember
		if upd == nil || upd.Chat.Type != models.ChatTypeChannel {
			return
		}

		kind, ok := classify(upd.OldChatMember, upd.NewChatMember)
		if !ok {
			return
		}

		subscriber := memberUser(upd.NewChatMember)
		if subscriber == nil {
			return
		}

		if err := profiles.Record(ctx, user.ProfileOf(subscriber)); err != nil {
			log.ErrorContext(ctx, "record subscriber", "user_id", subscriber.ID, "error", err)

			return
		}

		e := Event{ChatID: upd.Chat.ID, UserID: subscriber.ID, Kind: kind, HappenedAt: time.Unix(int64(upd.Date), 0)}
		if kind == Joined && upd.InviteLink != nil {
			e.InviteLink, e.InviteLinkName = upd.InviteLink.InviteLink, upd.InviteLink.Name
		}

		if err := events.AddEvent(ctx, e); err != nil {
			log.ErrorContext(ctx, "record membership event", "user_id", subscriber.ID, "kind", kind, "error", err)
		}
	}
}

// PostHandler returns a handler for channel_post updates. It records when each channel post
// was published; posts in other chats are ignored.
func PostHandler(posts Posts, log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, update *models.Update) {
		msg := update.ChannelPost
		if msg == nil || msg.Chat.Type != models.ChatTypeChannel {
			return
		}

		p := Post{ChatID: msg.Chat.ID, MessageID: int64(msg.ID), PostedAt: time.Unix(int64(msg.Date), 0)}
		if err := posts.AddPost(ctx, p); err != nil {
			log.ErrorContext(ctx, "record channel post", "message_id", msg.ID, "error", err)
		}
	}
}

// classify returns what a status change means for the subscription, and false when it moves
// the user neither in nor out: a promotion, a restriction, an unban.
func classify(old, upd models.ChatMember) (Kind, bool) {
	was, is := subscribed(old), subscribed(upd)

	switch {
	case !was && is:
		return Joined, true
	case was && upd.Type == models.ChatMemberTypeLeft:
		return Left, true
	case was && upd.Type == models.ChatMemberTypeBanned:
		return Removed, true
	default:
		return "", false
	}
}

// subscribed reports whether the status counts the user among the channel's subscribers.
func subscribed(m models.ChatMember) bool {
	switch m.Type {
	case models.ChatMemberTypeOwner, models.ChatMemberTypeAdministrator, models.ChatMemberTypeMember:
		return true
	case models.ChatMemberTypeRestricted:
		return m.Restricted != nil && m.Restricted.IsMember
	case models.ChatMemberTypeLeft, models.ChatMemberTypeBanned:
		return false
	}

	return false
}

// memberUser returns the user the status belongs to, or nil when Telegram left it out.
func memberUser(m models.ChatMember) *models.User {
	switch {
	case m.Owner != nil:
		return m.Owner.User
	case m.Administrator != nil:
		return &m.Administrator.User
	case m.Member != nil:
		return m.Member.User
	case m.Restricted != nil:
		return m.Restricted.User
	case m.Left != nil:
		return m.Left.User
	case m.Banned != nil:
		return m.Banned.User
	default:
		return nil
	}
}
