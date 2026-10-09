package user_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// fakeRecorder records the calls the middleware makes and fails each one with err.
type fakeRecorder struct {
	calls    []string
	profiles []user.Profile
	err      error
}

func (f *fakeRecorder) Record(_ context.Context, p user.Profile) error {
	f.calls = append(f.calls, fmt.Sprintf("names %s %s %d", p.FirstName, p.LastName, p.TelegramID))
	f.profiles = append(f.profiles, p)

	return f.err
}

func (f *fakeRecorder) SetPendingCommand(_ context.Context, id int64, command string) error {
	f.calls = append(f.calls, fmt.Sprintf("pending %s %d", command, id))

	return f.err
}

// isLabel stands in for menu.IsLabel with one known label.
func isLabel(text string) bool { return text == "🌐 Змінити мову" }

func privateMessage(text string) *models.Update {
	return &models.Update{Message: &models.Message{
		Text: text,
		Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate},
		From: &models.User{ID: 7, FirstName: "Olena", LastName: "Koval"},
	}}
}

func TestMiddleware(t *testing.T) {
	group := privateMessage("/giveaway")
	group.Message.Chat.Type = models.ChatTypeGroup

	asChannel := privateMessage("hello")
	asChannel.Message.Chat.Type = models.ChatTypeSupergroup
	asChannel.Message.SenderChat = &models.Chat{ID: -100, Type: models.ChatTypeChannel}

	edited := &models.Update{EditedMessage: privateMessage("/start").Message}

	callback := &models.Update{CallbackQuery: &models.CallbackQuery{
		From: models.User{ID: 9, FirstName: "Taras"},
	}}

	blocked := &models.Update{MyChatMember: &models.ChatMemberUpdated{
		From: models.User{ID: 9, FirstName: "Taras"},
	}}

	joinRequest := &models.Update{ChatJoinRequest: &models.ChatJoinRequest{
		From: models.User{ID: 9, FirstName: "Taras"},
	}}

	tests := []struct {
		name   string
		update *models.Update
		want   []string
	}{
		{"plain message", privateMessage("hello"), []string{"names Olena Koval 7"}},
		{"number keeps the step", privateMessage("5"), []string{"names Olena Koval 7"}},
		{"start clears", privateMessage("/start"), []string{"names Olena Koval 7", "pending  7"}},
		{
			"any command clears",
			privateMessage("/giveaway@read_wine_bot"),
			[]string{"names Olena Koval 7", "pending  7"},
		},
		{"menu label clears", privateMessage("🌐 Змінити мову"), []string{"names Olena Koval 7", "pending  7"}},
		{"group message records only", group, []string{"names Olena Koval 7"}},
		{"message as a channel", asChannel, nil},
		{"edited message records only", edited, []string{"names Olena Koval 7"}},
		{"callback query", callback, []string{"names Taras  9"}},
		{"bot blocked or added", blocked, []string{"names Taras  9"}},
		{"join request", joinRequest, []string{"names Taras  9"}},
		{"no sender", &models.Update{}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &fakeRecorder{}
			called := false
			next := func(context.Context, *bot.Bot, *models.Update) { called = true }

			user.Middleware(rec, isLabel, slog.New(slog.DiscardHandler))(next)(t.Context(), nil, tt.update)

			if !slices.Equal(rec.calls, tt.want) {
				t.Errorf("calls = %q, want %q", rec.calls, tt.want)
			}

			if !called {
				t.Error("next handler not called")
			}
		})
	}
}

func TestMiddlewareStoreError(t *testing.T) {
	rec := &fakeRecorder{err: errors.New("disk full")}
	called := false
	next := func(context.Context, *bot.Bot, *models.Update) { called = true }

	user.Middleware(rec, isLabel, slog.New(slog.DiscardHandler))(next)(t.Context(), nil, privateMessage("/start"))

	if !called {
		t.Error("next handler not called after a store error")
	}
}

func TestMiddlewareRecordsFullProfile(t *testing.T) {
	from := models.User{
		ID: 9, IsBot: true, FirstName: "Taras", LastName: "Shevchenko", Username: "kobzar",
		LanguageCode: "uk", IsPremium: true,
	}
	rec := &fakeRecorder{}
	next := func(context.Context, *bot.Bot, *models.Update) {}
	update := &models.Update{CallbackQuery: &models.CallbackQuery{From: from}}

	user.Middleware(rec, isLabel, slog.New(slog.DiscardHandler))(next)(t.Context(), nil, update)

	want := user.Profile{
		TelegramID: 9, IsBot: true, FirstName: "Taras", LastName: "Shevchenko", Username: "kobzar",
		LanguageCode: "uk", IsPremium: true,
	}
	if len(rec.profiles) != 1 || rec.profiles[0] != want {
		t.Errorf("recorded %+v, want %+v", rec.profiles, want)
	}
}
