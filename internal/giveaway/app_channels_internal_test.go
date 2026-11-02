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
		name     string
		tg       *channelAdmins
		want     []Channel
		wantWarn string
	}{
		{
			name: "only channels the viewer is an admin of, in store order",
			tg: &channelAdmins{
				fakeSubscriptionTelegram: &fakeSubscriptionTelegram{
					titles: all, usernames: map[int64]string{channelC: "c_wine"},
				},
				admins: map[int64][]int64{channelA: {viewer}, channelB: {7}, channelC: {7, viewer}},
			},
			want: []Channel{{ID: channelA, Title: "A"}, {ID: channelC, Title: "C", Username: "c_wine"}},
		},
		{
			name: "a failed admin check leaves the channel out",
			tg: &channelAdmins{
				fakeSubscriptionTelegram: titled(all),
				admins:                   map[int64][]int64{channelA: {viewer}, channelB: {viewer}},
				broken:                   map[int64]bool{channelB: true},
			},
			want:     []Channel{{ID: channelA, Title: "A"}},
			wantWarn: "check channel admin",
		},
		{
			name:     "an admin of none gets an empty list",
			tg:       &channelAdmins{fakeSubscriptionTelegram: titled(all)},
			want:     []Channel{},
			wantWarn: "",
		},
		{
			name: "a failed title read keeps the channel with an empty title",
			tg: &channelAdmins{
				fakeSubscriptionTelegram: titled(map[int64]string{channelA: "A"}),
				admins:                   map[int64][]int64{channelA: {viewer}, channelC: {viewer}},
			},
			want:     []Channel{{ID: channelA, Title: "A"}, {ID: channelC}},
			wantWarn: "read channel",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, logs := newChannelsApp(channelA, channelB, channelC)

			got, err := app.MyChannels(t.Context(), tc.tg, viewer)
			if err != nil {
				t.Fatal(err)
			}

			if len(got) != len(tc.want) {
				t.Fatalf("channels = %+v, want %+v", got, tc.want)
			}

			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("channels[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}

			if tc.wantWarn != "" && !strings.Contains(logs.String(), tc.wantWarn) {
				t.Errorf("logs = %q, want a warning %q", logs.String(), tc.wantWarn)
			}
		})
	}
}
