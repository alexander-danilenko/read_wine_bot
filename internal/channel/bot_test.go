package channel_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/channel"
)

// memBotChannels keeps bot_channels in memory and mirrors SetBotChannel: a status older than
// the saved one is ignored.
type memBotChannels map[int64]channel.BotChannel

func (m memBotChannels) SetBotChannel(_ context.Context, c channel.BotChannel) error {
	if saved, ok := m[c.ChatID]; !ok || !saved.ChangedAt.After(c.ChangedAt) {
		m[c.ChatID] = c
	}

	return nil
}

func botChange(chatType models.ChatType, upd models.ChatMember, at int) *models.Update {
	return &models.Update{MyChatMember: &models.ChatMemberUpdated{
		Chat:          models.Chat{ID: channelID, Type: chatType},
		Date:          at,
		NewChatMember: upd,
	}}
}

func TestBotHandler(t *testing.T) {
	tests := []struct {
		name    string
		updates []*models.Update
		want    *bool // nil for no row
	}{
		{"added as admin", []*models.Update{botChange(models.ChatTypeChannel, admin(), date)}, new(true)},
		{
			"removed",
			[]*models.Update{
				botChange(models.ChatTypeChannel, admin(), date),
				botChange(models.ChatTypeChannel, left(), date+1),
			},
			new(false),
		},
		{
			"older update arrives late",
			[]*models.Update{
				botChange(models.ChatTypeChannel, left(), date+1),
				botChange(models.ChatTypeChannel, admin(), date),
			},
			new(false),
		},
		{"group chat", []*models.Update{botChange(models.ChatTypeSupergroup, admin(), date)}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := memBotChannels{}
			h := channel.BotHandler(store, slog.New(slog.DiscardHandler))

			for _, u := range tt.updates {
				h(t.Context(), nil, u)
			}

			got, ok := store[channelID]
			if tt.want == nil {
				if ok {
					t.Fatalf("row = %+v, want none", got)
				}

				return
			}

			if !ok || got.BotIsAdmin != *tt.want {
				t.Errorf("row = %+v (saved %v), want bot_is_admin %v", got, ok, *tt.want)
			}

			if ok && got.ChangedAt.Before(time.Unix(date, 0)) {
				t.Errorf("changed_at = %v, want the update's date", got.ChangedAt)
			}
		})
	}
}
