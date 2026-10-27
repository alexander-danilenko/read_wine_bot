package giveaway

import (
	"context"
	"errors"
	"slices"
	"strings"
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

func TestReviewTextShowsSubscriptions(t *testing.T) {
	rv := newTestReviewer(t, newFakeReviewStore())
	g := Giveaway{ID: reviewID, ChatID: drawChannel, MessageID: drawPost}
	subs := Subscriptions{
		Channels: []ChannelInfo{
			{ID: drawChannel, Title: "Read Wine", Username: "read_wine"},
			{ID: otherChannel, Title: "Wine Club Kyiv Official Chat"},
		},
		Of: map[int64][]Subscription{
			ann().TelegramID: {Subscribed, SubscriptionUnknown},
			bob().TelegramID: {NotSubscribed, Subscribed},
			cid().TelegramID: {NotSubscribed, NotSubscribed},
		},
	}

	text := rv.ReviewText("en", g, Results{Winners: winnersOf(ann(), bob(), cid())}, subs)

	for _, want := range []string{
		`<a href="https://t.me/read_wine">Read Wine</a> · <a href="https://t.me/c/9999999999">Wine Club Kyiv Official Chat</a>`,
		"1. ✅ " + userLink(ann(), maxMessageLen) +
			` — <a href="https://t.me/read_wine">Read Wine</a>, ❔<a href="https://t.me/c/9999999999">Wine Club Kyiv Offi…</a>`,
		"2. ⚠️ " + userLink(bob(), maxMessageLen) + ` — <a href="https://t.me/c/9999999999">Wine Club Kyiv Offi…</a>`,
		"3. ⚠️ " + userLink(cid(), maxMessageLen) + "\n",
	} {
		if !strings.Contains(text+"\n", want) {
			t.Errorf("review = %q, want it to contain %q", text, want)
		}
	}
}

func TestReviewMarksUnknownWithoutOwnChannel(t *testing.T) {
	rv := newTestReviewer(t, newFakeReviewStore())
	g := Giveaway{ID: reviewID, ChatID: drawChannel, MessageID: drawPost}
	subs := Subscriptions{
		Channels: []ChannelInfo{{ID: otherChannel, Title: "Wine Club"}},
		Of:       map[int64][]Subscription{ann().TelegramID: {Subscribed}},
	}

	text := rv.ReviewText("en", g, Results{Winners: winnersOf(ann())}, subs)
	if !strings.Contains(text, "1. ❔ ") {
		t.Errorf("review = %q, want the unknown mark when the giveaway's channel could not be read", text)
	}
}

func TestReviewWithSubscriptionsFitsOneMessage(t *testing.T) {
	rv := newTestReviewer(t, newFakeReviewStore())
	g := Giveaway{ID: reviewID, ChatID: drawChannel, MessageID: drawPost}
	long := strings.Repeat("Ж", 64)
	title := strings.Repeat("Ш", 128)

	subs := Subscriptions{Of: map[int64][]Subscription{}}
	for i := range 3 {
		subs.Channels = append(subs.Channels, ChannelInfo{ID: drawChannel - int64(i), Title: title})
	}

	winners := make([]user.User, 0, maxWinners)
	for i := range int64(maxWinners) {
		winners = append(winners, user.User{TelegramID: i + 1, FirstName: long, LastName: long})
		subs.Of[i+1] = []Subscription{Subscribed, Subscribed, Subscribed}
	}

	text := rv.ReviewText("en", g, Results{Winners: winnersOf(winners...)}, subs)

	if n := visibleLen(text); n > maxMessageLen {
		t.Errorf("review is %d UTF-16 code units long, over %d", n, maxMessageLen)
	}

	if n := strings.Count(text, "tg://user?id="); n != maxWinners {
		t.Errorf("review links %d winners, want %d", n, maxWinners)
	}
}
