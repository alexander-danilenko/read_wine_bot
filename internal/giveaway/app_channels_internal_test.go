package giveaway

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
)

const (
	channelA int64 = -1001
	channelB int64 = -1002
	channelC int64 = -1003
	viewer   int64 = 42
)

// channelAdmins answers IsChannelAdmin from admins (channel -> its admins), fails it for the
// channels in broken, and reads titles from a fakeSubscriptionTelegram.
type channelAdmins struct {
	*fakeSubscriptionTelegram

	mu     sync.Mutex
	admins map[int64][]int64
	broken map[int64]bool
}

func (f *channelAdmins) IsChannelAdmin(_ context.Context, chatID, userID int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.broken[chatID] {
		return false, errTelegram
	}

	if slices.Contains(f.admins[chatID], userID) {
		return true, nil
	}

	return false, nil
}

// failingChannels fails to list the channels where the bot is an admin.
type failingChannels struct{}

func (failingChannels) AdminChannels(context.Context) ([]int64, error) { return nil, errTelegram }

func newChannelsApp(ids ...int64) (*App, *bytes.Buffer) {
	var logs bytes.Buffer

	log := slog.New(slog.NewTextHandler(&logs, nil))

	return &App{reviewer: &Reviewer{channels: fixedChannels(ids)}, log: log}, &logs
}

func titled(titles map[int64]string) *fakeSubscriptionTelegram {
	return &fakeSubscriptionTelegram{titles: titles}
}

func TestMyChannels(t *testing.T) {
	all := map[int64]string{channelA: "A", channelB: "B", channelC: "C"}

	tests := []struct {
		name         string
		tg           *channelAdmins
		wantMine     []Channel
		wantPartners []Channel
		wantWarn     string
	}{
		{
			name: "the viewer's channels are mine, the rest are partners, both in store order",
			tg: &channelAdmins{
				fakeSubscriptionTelegram: &fakeSubscriptionTelegram{
					titles: all, usernames: map[int64]string{channelB: "b_wine", channelC: "c_wine"},
				},
				admins: map[int64][]int64{channelA: {viewer}, channelB: {7}, channelC: {7, viewer}},
			},
			wantMine:     []Channel{{ID: channelA, Title: "A"}, {ID: channelC, Title: "C", Username: "c_wine"}},
			wantPartners: []Channel{{ID: channelB, Title: "B", Username: "b_wine"}},
		},
		{
			name: "a failed admin check makes the channel a partner",
			tg: &channelAdmins{
				fakeSubscriptionTelegram: titled(all),
				admins:                   map[int64][]int64{channelA: {viewer}, channelB: {viewer}},
				broken:                   map[int64]bool{channelB: true},
			},
			wantMine:     []Channel{{ID: channelA, Title: "A"}},
			wantPartners: []Channel{{ID: channelB, Title: "B"}, {ID: channelC, Title: "C"}},
			wantWarn:     "check channel admin",
		},
		{
			name:         "an admin of none gets every channel as a partner",
			tg:           &channelAdmins{fakeSubscriptionTelegram: titled(all)},
			wantMine:     []Channel{},
			wantPartners: []Channel{{ID: channelA, Title: "A"}, {ID: channelB, Title: "B"}, {ID: channelC, Title: "C"}},
		},
		{
			name: "a failed title read keeps my channel with an empty title and drops a partner",
			tg: &channelAdmins{
				fakeSubscriptionTelegram: titled(map[int64]string{channelA: "A"}),
				admins:                   map[int64][]int64{channelA: {viewer}, channelC: {viewer}},
			},
			wantMine:     []Channel{{ID: channelA, Title: "A"}, {ID: channelC}},
			wantPartners: []Channel{},
			wantWarn:     "read channel",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, logs := newChannelsApp(channelA, channelB, channelC)

			got, err := app.MyChannels(t.Context(), tc.tg, viewer)
			if err != nil {
				t.Fatal(err)
			}

			if !slices.Equal(got.Mine, tc.wantMine) {
				t.Errorf("mine = %+v, want %+v", got.Mine, tc.wantMine)
			}

			if !slices.Equal(got.Partners, tc.wantPartners) {
				t.Errorf("partners = %+v, want %+v", got.Partners, tc.wantPartners)
			}

			if tc.wantWarn != "" && !strings.Contains(logs.String(), tc.wantWarn) {
				t.Errorf("logs = %q, want a warning %q", logs.String(), tc.wantWarn)
			}
		})
	}
}

func TestMyChannelsStoreError(t *testing.T) {
	app := &App{reviewer: &Reviewer{channels: failingChannels{}}, log: slog.New(slog.DiscardHandler)}

	if _, err := app.MyChannels(
		t.Context(),
		&channelAdmins{fakeSubscriptionTelegram: titled(nil)},
		viewer,
	); err == nil {
		t.Error("err = nil, want the store error")
	}
}
