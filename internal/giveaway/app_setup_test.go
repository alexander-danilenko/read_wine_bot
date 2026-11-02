package giveaway_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// setup holds the giveaway rules over in-memory fakes, at a fixed Kyiv clock.
type setup struct {
	store *memStore
	users *fakeUsers
	tg    *fakeTelegram
	svc   *giveaway.Service
	ed    *giveaway.Editor
	now   time.Time
}

func newSetup(t *testing.T) *setup {
	t.Helper()

	texts, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}

	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}

	store := newMemStore()
	s := &setup{store: store, users: newFakeUsers(), tg: &fakeTelegram{admin: true}}
	s.now = time.Date(2026, 10, 9, 19, 55, 0, 0, kyiv)
	s.svc = giveaway.NewService(
		store,
		store,
		texts,
		kyiv,
		func() time.Time { return s.now },
		slog.New(slog.DiscardHandler),
	)
	s.ed = giveaway.NewEditor(s.svc, store, store, store, store, store, store)

	return s
}
