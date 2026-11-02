package giveaway

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

const (
	otherChannel  int64 = -1009999999999
	brokenChannel int64 = -1008888888888
)

var errTelegram = errors.New("telegram is down")

// fixedChannels is the list of channels where the bot is an admin.
type fixedChannels []int64

func (f fixedChannels) AdminChannels(context.Context) ([]int64, error) { return f, nil }

// fakeSubscriptionTelegram answers from maps. A channel missing from titles fails to read,
// and a user listed in failing fails every subscription check.
type fakeSubscriptionTelegram struct {
	mu         sync.Mutex
	titles     map[int64]string
	usernames  map[int64]string
	subscribed map[int64][]int64 // channel -> subscribers
	failing    map[int64]bool
}

func (f *fakeSubscriptionTelegram) Channel(_ context.Context, chatID int64) (ChannelInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	title, ok := f.titles[chatID]
	if !ok {
		return ChannelInfo{}, errTelegram
	}

	return ChannelInfo{ID: chatID, Title: title, Username: f.usernames[chatID]}, nil
}

func (f *fakeSubscriptionTelegram) IsSubscribed(_ context.Context, chatID, userID int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.failing[userID] {
		return false, errTelegram
	}

	if slices.Contains(f.subscribed[chatID], userID) {
		return true, nil
	}

	return false, nil
}

func winnersOf(us ...user.User) []Winner {
	ws := make([]Winner, 0, len(us))
	for i, u := range us {
		ws = append(ws, Winner{User: u, Place: i + 1})
	}

	return ws
}

func TestSubscriptions(t *testing.T) {
	rv := newTestReviewer(t, newFakeReviewStore())
	rv.channels = fixedChannels{otherChannel, drawChannel, brokenChannel}
	tg := &fakeSubscriptionTelegram{
		titles: map[int64]string{drawChannel: "Read Wine", otherChannel: "Wine Club"},
		subscribed: map[int64][]int64{
			drawChannel:  {ann().TelegramID},
			otherChannel: {ann().TelegramID, bob().TelegramID},
		},
		failing: map[int64]bool{cid().TelegramID: true},
	}
	g := Giveaway{ID: reviewID, ChatID: drawChannel}

	got := rv.Subscriptions(t.Context(), tg, g, winnersOf(ann(), bob(), cid()))

	if len(got.Channels) != 2 || got.Channels[0].ID != drawChannel || got.Channels[1].ID != otherChannel {
		t.Fatalf("channels = %+v, want the giveaway's channel, then the other; the unreadable one left out",
			got.Channels)
	}

	want := map[int64][]Subscription{
		ann().TelegramID: {Subscribed, Subscribed},
		bob().TelegramID: {NotSubscribed, Subscribed},
		cid().TelegramID: {SubscriptionUnknown, SubscriptionUnknown},
	}
	for id, w := range want {
		if g := got.Of[id]; len(g) != len(w) || g[0] != w[0] || g[1] != w[1] {
			t.Errorf("subscriptions of %d = %v, want %v", id, g, w)
		}
	}
}
