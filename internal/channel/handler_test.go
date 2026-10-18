package channel_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/channel"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

const (
	channelID int64 = -1_001_234_567_890 // channel IDs are negative and larger than int32
	fanID     int64 = 7_000_000_002
	date            = 1_760_000_000
)

// errConstraint stands in for a database constraint violation.
var errConstraint = errors.New("constraint violated")

// memStore keeps the log in memory and mirrors the database: the kind check rejects unknown
// kinds, and an event or post that is already saved is skipped, as ON CONFLICT DO NOTHING does.
type memStore struct {
	events   []channel.Event
	posts    []channel.Post
	profiles []user.Profile
	err      error // returned by Record
}

func (s *memStore) AddEvent(_ context.Context, e channel.Event) error {
	if !slices.Contains([]channel.Kind{channel.Joined, channel.Left, channel.Removed}, e.Kind) {
		return errConstraint
	}

	if !slices.ContainsFunc(s.events, func(o channel.Event) bool {
		return o.ChatID == e.ChatID && o.UserID == e.UserID && o.Kind == e.Kind && o.HappenedAt.Equal(e.HappenedAt)
	}) {
		s.events = append(s.events, e)
	}

	return nil
}

func (s *memStore) AddPost(_ context.Context, p channel.Post) error {
	if !slices.ContainsFunc(s.posts, func(o channel.Post) bool {
		return o.ChatID == p.ChatID && o.MessageID == p.MessageID
	}) {
		s.posts = append(s.posts, p)
	}

	return nil
}

func (s *memStore) Record(_ context.Context, p user.Profile) error {
	if s.err != nil {
		return s.err
	}

	s.profiles = append(s.profiles, p)

	return nil
}

func fan() *models.User {
	return &models.User{ID: fanID, FirstName: "Olena", Username: "olena"}
}

func member() models.ChatMember {
	return models.ChatMember{Type: models.ChatMemberTypeMember, Member: &models.ChatMemberMember{User: fan()}}
}

func left() models.ChatMember {
	return models.ChatMember{Type: models.ChatMemberTypeLeft, Left: &models.ChatMemberLeft{User: fan()}}
}

func banned() models.ChatMember {
	return models.ChatMember{Type: models.ChatMemberTypeBanned, Banned: &models.ChatMemberBanned{User: fan()}}
}

func admin() models.ChatMember {
	return models.ChatMember{
		Type: models.ChatMemberTypeAdministrator, Administrator: &models.ChatMemberAdministrator{User: *fan()},
	}
}

func restricted(isMember bool) models.ChatMember {
	return models.ChatMember{
		Type:       models.ChatMemberTypeRestricted,
		Restricted: &models.ChatMemberRestricted{User: fan(), IsMember: isMember},
	}
}

func change(old, upd models.ChatMember) *models.Update {
	return &models.Update{ChatMember: &models.ChatMemberUpdated{
		Chat:          models.Chat{ID: channelID, Type: models.ChatTypeChannel},
		From:          *fan(),
		Date:          date,
		OldChatMember: old,
		NewChatMember: upd,
	}}
}

func TestMemberHandler(t *testing.T) {
	tests := []struct {
		name     string
		old, upd models.ChatMember
		want     channel.Kind // "" for no event
	}{
		{"subscribes", left(), member(), channel.Joined},
		{"subscribes after an unban", banned(), member(), channel.Joined},
		{"restricted user becomes a member", restricted(false), member(), channel.Joined},
		{"unsubscribes", member(), left(), channel.Left},
		{"admin unsubscribes", admin(), left(), channel.Left},
		{"restricted member unsubscribes", restricted(true), left(), channel.Left},
		{"removed by an admin", member(), banned(), channel.Removed},
		{"promoted to admin", member(), admin(), ""},
		{"restricted while subscribed", member(), restricted(true), ""},
		{"unbanned without subscribing", banned(), left(), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &memStore{}
			channel.MemberHandler(store, store, slog.New(slog.DiscardHandler))(t.Context(), nil, change(tt.old, tt.upd))

			if tt.want == "" {
				if len(store.events) != 0 || len(store.profiles) != 0 {
					t.Fatalf("events %v, profiles %v, want none", store.events, store.profiles)
				}

				return
			}

			want := []channel.Event{{
				ChatID: channelID, UserID: fanID, Kind: tt.want, HappenedAt: time.Unix(date, 0),
			}}
			if !slices.EqualFunc(store.events, want, sameEvent) {
				t.Errorf("events = %v, want %v", store.events, want)
			}

			if len(store.profiles) != 1 || store.profiles[0].TelegramID != fanID ||
				store.profiles[0].Username != "olena" {
				t.Errorf("profiles = %v, want the subscriber's", store.profiles)
			}
		})
	}
}

func sameEvent(a, b channel.Event) bool {
	return a.ChatID == b.ChatID && a.UserID == b.UserID && a.Kind == b.Kind && a.HappenedAt.Equal(b.HappenedAt) &&
		a.InviteLink == b.InviteLink && a.InviteLinkName == b.InviteLinkName
}

func TestMemberHandlerInviteLink(t *testing.T) {
	link := &models.ChatInviteLink{InviteLink: "https://t.me/+AbCd…", Name: "Ad at wine_daily"}

	join := change(left(), member())
	join.ChatMember.InviteLink = link

	leave := change(member(), left())
	leave.ChatMember.InviteLink = link // Telegram sends it only on joins; the handler ignores it otherwise

	store := &memStore{}
	h := channel.MemberHandler(store, store, slog.New(slog.DiscardHandler))
	h(t.Context(), nil, join)
	h(t.Context(), nil, leave)

	want := []channel.Event{
		{
			ChatID: channelID, UserID: fanID, Kind: channel.Joined, HappenedAt: time.Unix(date, 0),
			InviteLink: link.InviteLink, InviteLinkName: link.Name,
		},
		{ChatID: channelID, UserID: fanID, Kind: channel.Left, HappenedAt: time.Unix(date, 0)},
	}
	if !slices.EqualFunc(store.events, want, sameEvent) {
		t.Errorf("events = %v, want %v", store.events, want)
	}
}

func TestMemberHandlerSkips(t *testing.T) {
	group := change(left(), member())
	group.ChatMember.Chat.Type = models.ChatTypeSupergroup

	t.Run("group chat", func(t *testing.T) {
		store := &memStore{}
		channel.MemberHandler(store, store, slog.New(slog.DiscardHandler))(t.Context(), nil, group)

		if len(store.events) != 0 || len(store.profiles) != 0 {
			t.Errorf("events %v, profiles %v, want none", store.events, store.profiles)
		}
	})

	t.Run("delivered twice", func(t *testing.T) {
		store := &memStore{}
		h := channel.MemberHandler(store, store, slog.New(slog.DiscardHandler))
		h(t.Context(), nil, change(left(), member()))
		h(t.Context(), nil, change(left(), member()))

		if len(store.events) != 1 {
			t.Errorf("events = %v, want one", store.events)
		}
	})

	t.Run("profile not saved", func(t *testing.T) {
		store := &memStore{err: errors.New("database down")}
		channel.MemberHandler(store, store, slog.New(slog.DiscardHandler))(t.Context(), nil, change(left(), member()))

		if len(store.events) != 0 {
			t.Errorf("events = %v, want none: the event needs the users row", store.events)
		}
	})
}

func TestPostHandler(t *testing.T) {
	post := func(chatType models.ChatType) *models.Update {
		return &models.Update{ChannelPost: &models.Message{
			ID: 42, Date: date, Chat: models.Chat{ID: channelID, Type: chatType},
		}}
	}

	store := &memStore{}
	h := channel.PostHandler(store, slog.New(slog.DiscardHandler))
	h(t.Context(), nil, post(models.ChatTypeChannel))
	h(t.Context(), nil, post(models.ChatTypeChannel))
	h(t.Context(), nil, post(models.ChatTypeSupergroup))

	want := []channel.Post{{ChatID: channelID, MessageID: 42, PostedAt: time.Unix(date, 0)}}
	if !slices.EqualFunc(store.posts, want, func(a, b channel.Post) bool {
		return a.ChatID == b.ChatID && a.MessageID == b.MessageID && a.PostedAt.Equal(b.PostedAt)
	}) {
		t.Errorf("posts = %v, want %v", store.posts, want)
	}
}
