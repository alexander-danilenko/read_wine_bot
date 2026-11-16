package giveaway

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/go-telegram/bot/models"
)

const (
	primaryChannel int64 = -1000
	channelA       int64 = -1001
	channelB       int64 = -1002
	channelC       int64 = -1003
	viewer         int64 = 42
)

// channelMembers answers ChatMember from members (channel -> user -> record), where a user
// missing from a channel has left it, and BotMember from bot (channel -> the bot's record). A
// channel in brokenBot fails BotMember. Titles come from a fakeSubscriptionTelegram.
type channelMembers struct {
	*fakeSubscriptionTelegram

	mu        sync.Mutex
	members   map[int64]map[int64]models.ChatMember
	bot       map[int64]models.ChatMember
	brokenBot map[int64]bool
	memberErr error // fails every ChatMember when set
}

func (f *channelMembers) ChatMember(_ context.Context, chatID, userID int64) (models.ChatMember, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.memberErr != nil {
		return models.ChatMember{}, f.memberErr
	}

	if m, ok := f.members[chatID][userID]; ok {
		return m, nil
	}

	return models.ChatMember{Type: models.ChatMemberTypeLeft, Left: &models.ChatMemberLeft{}}, nil
}

func (f *channelMembers) BotMember(_ context.Context, chatID int64) (models.ChatMember, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.brokenBot[chatID] {
		return models.ChatMember{}, errTelegram
	}

	return f.bot[chatID], nil
}

// failingChannels fails to list the channels where the bot is an admin.
type failingChannels struct{}

func (failingChannels) AdminChannels(context.Context) ([]int64, error) { return nil, errTelegram }

func newChannelsApp(ids ...int64) (*App, *bytes.Buffer) {
	var logs bytes.Buffer

	log := slog.New(slog.NewTextHandler(&logs, nil))

	return &App{reviewer: &Reviewer{channels: fixedChannels(ids)}, primary: primaryChannel, log: log}, &logs
}

func titled(titles map[int64]string) *fakeSubscriptionTelegram {
	return &fakeSubscriptionTelegram{titles: titles}
}

func TestMyChannels(t *testing.T) {
	all := map[int64]string{primaryChannel: "Read Wine", channelA: "A", channelB: "B", channelC: "C"}
	owner := models.ChatMember{Type: models.ChatMemberTypeOwner, Owner: &models.ChatMemberOwner{}}
	manageOnly := admin(models.ChatMemberAdministrator{CanManageChat: true})
	poster := admin(models.ChatMemberAdministrator{CanManageChat: true, CanPostMessages: true})

	tests := []struct {
		name         string
		asPartner    bool
		tg           *channelMembers
		wantMine     []Channel
		wantPartners []Channel
		wantWarn     string
	}{
		{
			name: "a full admin who also admins a partner channel gets the primary and every partner",
			tg: &channelMembers{
				fakeSubscriptionTelegram: titled(all),
				members: map[int64]map[int64]models.ChatMember{
					primaryChannel: {viewer: owner}, channelA: {viewer: owner},
				},
			},
			wantMine: []Channel{{ID: primaryChannel, Title: "Read Wine"}},
			wantPartners: []Channel{
				{ID: channelA, Title: "A"}, {ID: channelB, Title: "B"}, {ID: channelC, Title: "C"},
			},
		},
		{
			name: "a read-only admin gets the same lists as a full admin",
			tg: &channelMembers{
				fakeSubscriptionTelegram: titled(all),
				members:                  map[int64]map[int64]models.ChatMember{primaryChannel: {viewer: manageOnly}},
			},
			wantMine: []Channel{{ID: primaryChannel, Title: "Read Wine"}},
			wantPartners: []Channel{
				{ID: channelA, Title: "A"}, {ID: channelB, Title: "B"}, {ID: channelC, Title: "C"},
			},
		},
		{
			name: "a partner gets the channels they admin with the bot's extra rights, and no partners",
			tg: &channelMembers{
				fakeSubscriptionTelegram: titled(all),
				members: map[int64]map[int64]models.ChatMember{
					channelA: {viewer: owner}, channelC: {viewer: manageOnly}, channelB: {7: owner},
				},
				bot: map[int64]models.ChatMember{channelA: manageOnly, channelC: poster},
			},
			wantMine: []Channel{
				{ID: channelA, Title: "A", ExtraRights: []string{}},
				{ID: channelC, Title: "C", ExtraRights: []string{"can_post_messages"}},
			},
			wantPartners: []Channel{},
		},
		{
			name:      "an owner asking for the partner view gets the primary channel first, then their partner channels",
			asPartner: true,
			tg: &channelMembers{
				fakeSubscriptionTelegram: titled(all),
				members: map[int64]map[int64]models.ChatMember{
					primaryChannel: {viewer: owner}, channelA: {viewer: owner},
				},
				bot: map[int64]models.ChatMember{primaryChannel: poster, channelA: manageOnly},
			},
			wantMine: []Channel{
				{ID: primaryChannel, Title: "Read Wine", ExtraRights: []string{"can_post_messages"}},
				{ID: channelA, Title: "A", ExtraRights: []string{}},
			},
			wantPartners: []Channel{},
		},
		{
			name:         "a partner who admins nothing gets two empty lists",
			tg:           &channelMembers{fakeSubscriptionTelegram: titled(all)},
			wantMine:     []Channel{},
			wantPartners: []Channel{},
		},
		{
			name: "a failed read of the bot's rights keeps the channel with unknown extra rights",
			tg: &channelMembers{
				fakeSubscriptionTelegram: titled(all),
				members: map[int64]map[int64]models.ChatMember{
					channelA: {viewer: owner}, channelC: {viewer: owner},
				},
				bot:       map[int64]models.ChatMember{channelA: manageOnly},
				brokenBot: map[int64]bool{channelC: true},
			},
			wantMine: []Channel{
				{ID: channelA, Title: "A", ExtraRights: []string{}},
				{ID: channelC, Title: "C"},
			},
			wantPartners: []Channel{},
			wantWarn:     "read bot rights",
		},
		{
			name: "a failed title read keeps a partner's own channel with an empty title",
			tg: &channelMembers{
				fakeSubscriptionTelegram: titled(map[int64]string{channelA: "A"}),
				members: map[int64]map[int64]models.ChatMember{
					channelA: {viewer: owner}, channelC: {viewer: owner},
				},
			},
			wantMine: []Channel{
				{ID: channelA, Title: "A", ExtraRights: []string{}},
				{ID: channelC, ExtraRights: []string{}},
			},
			wantPartners: []Channel{},
			wantWarn:     "read channel",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, logs := newChannelsApp(channelA, primaryChannel, channelB, channelC)

			got, err := app.MyChannels(t.Context(), tc.tg, viewer, tc.asPartner)
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(got.Mine, tc.wantMine) {
				t.Errorf("mine = %#v, want %#v", got.Mine, tc.wantMine)
			}

			if !reflect.DeepEqual(got.Partners, tc.wantPartners) {
				t.Errorf("partners = %#v, want %#v", got.Partners, tc.wantPartners)
			}

			if tc.wantWarn != "" && !strings.Contains(logs.String(), tc.wantWarn) {
				t.Errorf("logs = %q, want a warning %q", logs.String(), tc.wantWarn)
			}
		})
	}
}

func TestMyChannelsPrimaryAdminWithUnreadablePartner(t *testing.T) {
	app, logs := newChannelsApp(primaryChannel, channelA)
	tg := &channelMembers{
		fakeSubscriptionTelegram: titled(map[int64]string{primaryChannel: "Read Wine"}),
		members: map[int64]map[int64]models.ChatMember{
			primaryChannel: {viewer: {Type: models.ChatMemberTypeOwner, Owner: &models.ChatMemberOwner{}}},
		},
	}

	got, err := app.MyChannels(t.Context(), tg, viewer, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Partners) != 0 || !strings.Contains(logs.String(), "read channel") {
		t.Errorf("partners = %+v, logs %q; want the unreadable partner dropped and logged", got.Partners, logs)
	}
}

func TestMyChannelsErrors(t *testing.T) {
	tests := []struct {
		name     string
		channels Channels
		tg       *channelMembers
	}{
		{"the store fails", failingChannels{}, &channelMembers{fakeSubscriptionTelegram: titled(nil)}},
		{
			"the role lookup fails",
			fixedChannels{channelA},
			&channelMembers{fakeSubscriptionTelegram: titled(nil), memberErr: errTelegram},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := &App{reviewer: &Reviewer{channels: tc.channels}, log: slog.New(slog.DiscardHandler)}

			if _, err := app.MyChannels(t.Context(), tc.tg, viewer, false); err == nil {
				t.Error("err = nil, want an error")
			}
		})
	}
}
