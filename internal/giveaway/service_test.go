package giveaway_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

var errBoom = errors.New("boom")

// fakeStore holds at most one giveaway and fails the call named in failOn.
type fakeStore struct {
	existing *giveaway.Giveaway
	joined   map[int64]bool
	failOn   string
}

func (f *fakeStore) Post(_ context.Context, chatID, messageID int64) (giveaway.Giveaway, error) {
	if f.failOn == "find" {
		return giveaway.Giveaway{}, errBoom
	}

	if f.existing == nil || f.existing.ChatID != chatID || f.existing.MessageID != messageID {
		return giveaway.Giveaway{}, giveaway.ErrNotFound
	}

	return *f.existing, nil
}

func (*fakeStore) Publish(context.Context, uint) error { return nil }

func (*fakeStore) SaveDraft(context.Context, *giveaway.Giveaway) error { return nil }

func (*fakeStore) Draft(context.Context, int64) (giveaway.Giveaway, error) {
	return giveaway.Giveaway{}, giveaway.ErrNotFound
}

func (*fakeStore) DeleteDraft(context.Context, int64) error { return nil }

func (f *fakeStore) Join(_ context.Context, _ uint, telegramID int64) (bool, error) {
	if f.failOn == "join" {
		return false, errBoom
	}

	if f.joined == nil {
		f.joined = map[int64]bool{}
	}

	added := !f.joined[telegramID]
	f.joined[telegramID] = true

	return added, nil
}

// fakeTelegram answers the admin and rights checks and records the attached button label.
type fakeTelegram struct {
	admin       bool
	adminErr    error
	noEdit      bool // the bot may not edit the channel's posts
	attachErr   error
	removeErr   error
	label       string
	removed     int
	adminChecks int
	attached    int
	attachedTo  int64 // the post of the last attach
	removedFrom int64 // the post of the last removal
}

func (f *fakeTelegram) IsChannelAdmin(context.Context, int64, int64) (bool, error) {
	f.adminChecks++

	return f.admin, f.adminErr
}

func (f *fakeTelegram) CanRunGiveaways(context.Context, int64) (bool, error) { return !f.noEdit, nil }

func (f *fakeTelegram) AttachButton(_ context.Context, _, messageID int64, label string) error {
	if f.attachErr != nil {
		return f.attachErr
	}

	f.label = label
	f.attached++
	f.attachedTo = messageID

	return nil
}

func (f *fakeTelegram) RemoveButton(_ context.Context, _, messageID int64) error {
	if f.removeErr != nil {
		return f.removeErr
	}

	f.removed++
	f.removedFrom = messageID

	return nil
}

func newService(t *testing.T, store *fakeStore) *giveaway.Service {
	t.Helper()

	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	return giveaway.NewService(store, store, nil, texts, time.UTC, time.Now, slog.New(slog.DiscardHandler))
}

func TestServiceJoin(t *testing.T) {
	existing := &giveaway.Giveaway{ID: 1, ChatID: channelID, MessageID: 42, Language: "uk", Published: true}
	store := &fakeStore{existing: existing}
	svc := newService(t, store)

	for i, want := range []giveaway.JoinResult{giveaway.Joined, giveaway.AlreadyJoined} {
		got, lang := svc.Join(t.Context(), channelID, 42, fanID)
		if got != want || lang != "uk" {
			t.Errorf("Join #%d = %v, %q; want %v, \"uk\"", i+1, got, lang, want)
		}
	}

	if got, lang := svc.Join(t.Context(), channelID, 43, fanID); got != giveaway.JoinFailed || lang != "" {
		t.Errorf("Join on a post without a giveaway = %v, %q; want JoinFailed, \"\"", got, lang)
	}

	existing.Published = false // cancelled: the post keeps a stray button
	if got, lang := svc.Join(t.Context(), channelID, 42, 7_000_000_003); got != giveaway.JoinFailed || lang != "" {
		t.Errorf("Join on a cancelled giveaway = %v, %q; want JoinFailed, \"\"", got, lang)
	}

	existing.Published = true
	store.failOn = "join"
	if got, lang := svc.Join(t.Context(), channelID, 42, fanID); got != giveaway.JoinFailed || lang != "uk" {
		t.Errorf("Join with a store error = %v, %q; want JoinFailed, \"uk\"", got, lang)
	}
}

func TestServiceJoinAfterEnd(t *testing.T) {
	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	ends := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	finished := ends.Add(time.Minute)

	tests := []struct {
		name     string
		endsAt   *time.Time
		finished *time.Time
		now      time.Time
		want     giveaway.JoinResult
	}{
		{"a second before the end", &ends, nil, ends.Add(-time.Second), giveaway.Joined},
		{"exactly at the end", &ends, nil, ends, giveaway.Ended},
		{"after the end, not drawn yet", &ends, nil, ends.Add(30 * time.Second), giveaway.Ended},
		{"finished", &ends, &finished, ends.Add(time.Hour), giveaway.Ended},
		{"no end time", nil, nil, ends.Add(time.Hour), giveaway.Joined},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{existing: &giveaway.Giveaway{
				ID: 1, ChatID: channelID, MessageID: 42, Language: "uk", Published: true,
				EndsAt: tt.endsAt, FinishedAt: tt.finished,
			}}
			now := func() time.Time { return tt.now }
			svc := giveaway.NewService(store, store, nil, texts, time.UTC, now, slog.New(slog.DiscardHandler))

			got, lang := svc.Join(t.Context(), channelID, 42, fanID)
			if got != tt.want || lang != "uk" {
				t.Errorf("Join = %v, %q; want %v, \"uk\"", got, lang, tt.want)
			}

			if joined := store.joined[fanID]; joined != (tt.want == giveaway.Joined) {
				t.Errorf("participant recorded = %v, want %v", joined, tt.want == giveaway.Joined)
			}
		})
	}
}
