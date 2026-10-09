package giveaway

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-telegram/bot"
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
