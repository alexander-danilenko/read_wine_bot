package giveaway

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-telegram/bot"

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
