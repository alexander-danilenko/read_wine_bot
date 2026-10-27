package giveaway

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

func TestIgnoreNotModified(t *testing.T) {
	// go-telegram/bot wraps a 400 answer as "<ErrorBadRequest>, <description>".
	notModified := fmt.Errorf("%w, %s", bot.ErrorBadRequest,
		"Bad Request: message is not modified: specified new message content and reply markup are exactly the same")
	noRights := fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: message can't be edited")
	forbidden := fmt.Errorf("%w, %s", bot.ErrorForbidden, "Forbidden: bot is not a member of the channel chat")

	tests := []struct {
		name string
		err  error
		want error
	}{
		{"no error", nil, nil},
		{"button already there", notModified, nil},
		{"cannot edit", noRights, noRights},
		{"not in channel", forbidden, forbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ignoreNotModified(tt.err); !errors.Is(got, tt.want) || (got == nil) != (tt.want == nil) {
				t.Errorf("ignoreNotModified(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIgnoreGone(t *testing.T) {
	gone := fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: message to edit not found")
	// Telegram answers this for a deleted channel post.
	invalid := fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: MESSAGE_ID_INVALID")
	notModified := fmt.Errorf("%w, %s", bot.ErrorBadRequest,
		"Bad Request: message is not modified: specified new message content and reply markup are exactly the same")
	noRights := fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: message can't be edited")
	forbidden := fmt.Errorf("%w, %s", bot.ErrorForbidden, "Forbidden: bot is not a member of the channel chat")

	tests := []struct {
		name string
		err  error
		want error
	}{
		{"no error", nil, nil},
		{"post deleted", gone, nil},
		{"channel post deleted", invalid, nil},
		{"button already there", notModified, notModified},
		{"cannot edit", noRights, noRights},
		{"not in channel", forbidden, forbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ignoreGone(tt.err); !errors.Is(got, tt.want) || (got == nil) != (tt.want == nil) {
				t.Errorf("ignoreGone(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestMarkGone(t *testing.T) {
	gone := fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: message to edit not found")
	noRights := fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: message can't be edited")

	if got := markGone(gone); !errors.Is(got, ErrPostGone) {
		t.Errorf("markGone(deleted post) = %v, want ErrPostGone", got)
	}

	invalid := fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: MESSAGE_ID_INVALID")
	if got := markGone(invalid); !errors.Is(got, ErrPostGone) {
		t.Errorf("markGone(deleted channel post) = %v, want ErrPostGone", got)
	}

	if got := markGone(noRights); errors.Is(got, ErrPostGone) || !errors.Is(got, noRights) {
		t.Errorf("markGone(no rights) = %v, want it unchanged", got)
	}

	if got := markGone(nil); got != nil {
		t.Errorf("markGone(nil) = %v, want nil", got)
	}
}

func TestJoinText(t *testing.T) {
	tests := []struct {
		result JoinResult
		want   i18n.Key
	}{
		{Joined, i18n.GiveawayJoined},
		{AlreadyJoined, i18n.GiveawayAlreadyJoined},
		{JoinFailed, i18n.GiveawayJoinFailed},
		{Ended, i18n.GiveawayEnded},
	}

	for _, tt := range tests {
		if got := joinText(tt.result); got != tt.want {
			t.Errorf("joinText(%v) = %q, want %q", tt.result, got, tt.want)
		}
	}
}

func TestParsePage(t *testing.T) {
	tests := []struct {
		data string
		page int
		ok   bool
	}{
		{FinishedCallbackPrefix + "1", 1, true},
		{FinishedCallbackPrefix + "12", 12, true},
		{FinishedCallbackPrefix + "0", 0, false},
		{FinishedCallbackPrefix + "-1", 0, false},
		{FinishedCallbackPrefix + "x", 0, false},
		{FinishedCallbackPrefix + "99999999999", 0, false},
		{ListCallbackPrefix + "1", 0, false},
	}
	for _, tt := range tests {
		if page, ok := parsePage(tt.data, FinishedCallbackPrefix); page != tt.page || ok != tt.ok {
			t.Errorf("parsePage(%q) = %d, %v; want %d, %v", tt.data, page, ok, tt.page, tt.ok)
		}
	}
}

func TestPageCallbackPrefixes(t *testing.T) {
	others := []string{EditCallbackPrefix, SetupCallbackPrefix, ReviewCallbackPrefix, JoinCallback}
	for _, prefix := range []string{ListCallbackPrefix, FinishedCallbackPrefix} {
		for _, other := range append(others, ListCallbackPrefix, FinishedCallbackPrefix) {
			if other != prefix && (strings.HasPrefix(other, prefix) || strings.HasPrefix(prefix, other)) {
				t.Errorf("page prefix %q and %q overlap", prefix, other)
			}
		}

		if data := prefix + "2147483647"; len(data) > 64 {
			t.Errorf("callback data %q is %d bytes, over Telegram's 64", data, len(data))
		}
	}
}

func TestCanRunGiveaways(t *testing.T) {
	admin := func(edit, post bool) models.ChatMember {
		return models.ChatMember{
			Type:          models.ChatMemberTypeAdministrator,
			Administrator: &models.ChatMemberAdministrator{CanEditMessages: edit, CanPostMessages: post},
		}
	}

	tests := []struct {
		name string
		m    models.ChatMember
		want bool
	}{
		{"both rights", admin(true, true), true},
		{
			"the rights the bot requests",
			admin(requiredRights().CanEditMessages, requiredRights().CanPostMessages),
			true,
		},
		{"no post right", admin(true, false), false},
		{"no edit right", admin(false, true), false},
		{
			"not an admin",
			models.ChatMember{Type: models.ChatMemberTypeMember, Member: &models.ChatMemberMember{}},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canRunGiveaways(tt.m); got != tt.want {
				t.Errorf("canRunGiveaways() = %v, want %v", got, tt.want)
			}
		})
	}
}
