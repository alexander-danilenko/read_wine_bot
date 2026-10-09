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
	existing  *giveaway.Giveaway
	created   *giveaway.Giveaway
	joined    map[int64]bool
	createErr error
	failOn    string
}

func (f *fakeStore) Find(_ context.Context, chatID, messageID int64) (giveaway.Giveaway, error) {
	if f.failOn == "find" {
		return giveaway.Giveaway{}, errBoom
	}

	if f.existing == nil || f.existing.ChatID != chatID || f.existing.MessageID != messageID {
		return giveaway.Giveaway{}, giveaway.ErrNotFound
	}

	return *f.existing, nil
}

func (f *fakeStore) Create(_ context.Context, g *giveaway.Giveaway) error {
	if f.createErr != nil {
		return f.createErr
	}

	f.created = g

	return nil
}

func (*fakeStore) SaveDraft(context.Context, *giveaway.Draft) error { return nil }

func (*fakeStore) Draft(context.Context, int64) (giveaway.Draft, error) {
	return giveaway.Draft{}, giveaway.ErrNotFound
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
	label       string
	adminChecks int
	attached    int
}

func (f *fakeTelegram) IsChannelAdmin(context.Context, int64, int64) (bool, error) {
	f.adminChecks++

	return f.admin, f.adminErr
}

func (f *fakeTelegram) CanEditPosts(context.Context, int64) (bool, error) { return !f.noEdit, nil }

func (f *fakeTelegram) AttachButton(_ context.Context, _, _ int64, label string) error {
	if f.attachErr != nil {
		return f.attachErr
	}

	f.label = label
	f.attached++

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
	existing := &giveaway.Giveaway{ID: 1, ChatID: channelID, MessageID: 42, Language: "uk"}
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

	store.failOn = "join"
	if got, lang := svc.Join(t.Context(), channelID, 42, fanID); got != giveaway.JoinFailed || lang != "uk" {
		t.Errorf("Join with a store error = %v, %q; want JoinFailed, \"uk\"", got, lang)
	}
}
