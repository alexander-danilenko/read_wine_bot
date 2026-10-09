package giveaway_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/alexander-danilenko/read_wine_bot/internal/db"
	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

const (
	channelID int64 = -1_001_234_567_890 // channel IDs are negative and larger than int32
	adminID   int64 = 7_000_000_001
	fanID     int64 = 7_000_000_002
)

// newStore opens a fresh database file the way the bot does, so foreign keys are on, and
// saves the admin and the fan as users.
func newStore(t *testing.T) (*giveaway.Store, *gorm.DB) {
	t.Helper()

	conn, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "bot.db"))
	if err != nil {
		t.Fatal(err)
	}

	sqlDB, err := conn.DB()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = sqlDB.Close() })

	users := user.NewStore(conn)
	for _, id := range []int64{adminID, fanID} {
		if err := users.SaveNames(t.Context(), id, "Name", ""); err != nil {
			t.Fatal(err)
		}
	}

	return giveaway.NewStore(conn), conn
}

// publish saves g as the admin's draft and publishes it, the way Confirm does.
func publish(t *testing.T, store *giveaway.Store, g *giveaway.Giveaway) giveaway.Giveaway {
	t.Helper()

	ctx := t.Context()
	if err := store.SaveDraft(ctx, g); err != nil {
		t.Fatal(err)
	}

	d, err := store.Draft(ctx, g.CreatedBy)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Publish(ctx, d.ID); err != nil {
		t.Fatal(err)
	}

	d.Published = true

	return d
}

func TestFindSeesOnlyPublished(t *testing.T) {
	store, _ := newStore(t)
	ctx := t.Context()

	if err := store.SaveDraft(
		ctx,
		&giveaway.Giveaway{ChatID: channelID, MessageID: 41, CreatedBy: fanID},
	); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Find(ctx, channelID, 41); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("Find a draft: err = %v, want ErrNotFound", err)
	}

	published := publish(
		t,
		store,
		&giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, Language: "uk"},
	)

	got, err := store.Find(ctx, channelID, 42)
	if err != nil {
		t.Fatal(err)
	}

	if got.ID != published.ID || !got.Published || got.Language != "uk" {
		t.Errorf("Find = %+v, want the published giveaway %d", got, published.ID)
	}
}

func TestPublish(t *testing.T) {
	store, _ := newStore(t)
	ctx := t.Context()

	g := publish(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})

	if err := store.Publish(ctx, g.ID); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("second Publish: err = %v, want ErrNotFound", err)
	}

	if _, err := store.Draft(ctx, adminID); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("Draft after Publish: err = %v, want ErrNotFound", err)
	}
}

func TestSaveDraft(t *testing.T) {
	store, _ := newStore(t)
	ctx := t.Context()
	ends := time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC)

	if err := store.SaveDraft(ctx, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: fanID}); err != nil {
		t.Fatal(err)
	}

	// Another admin forwarding the same post takes the unpublished row over.
	d := &giveaway.Giveaway{
		ChatID: channelID, MessageID: 42, CreatedBy: adminID, Language: "uk",
		EndsAt: &ends, Winners: 3, ButtonText: "Беру участь",
	}
	if err := store.SaveDraft(ctx, d); err != nil {
		t.Fatal(err)
	}

	got, err := store.Draft(ctx, adminID)
	if err != nil {
		t.Fatal(err)
	}

	if got.EndsAt == nil || !got.EndsAt.Equal(ends) || got.Winners != 3 || got.ButtonText != "Беру участь" ||
		got.Published {
		t.Errorf("draft = %+v; want ends %v, 3 winners, the button text, unpublished", got, ends)
	}

	if _, err := store.Draft(ctx, fanID); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("Draft of the first admin: err = %v, want ErrNotFound", err)
	}
}

func TestSaveDraftOnPublishedPost(t *testing.T) {
	store, _ := newStore(t)
	ctx := t.Context()

	publish(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, ButtonText: "Go"})

	err := store.SaveDraft(ctx, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: fanID})
	if !errors.Is(err, giveaway.ErrExists) {
		t.Errorf("SaveDraft on a published post: err = %v, want ErrExists", err)
	}

	if g, err := store.Find(ctx, channelID, 42); err != nil || g.CreatedBy != adminID || g.ButtonText != "Go" {
		t.Errorf("published giveaway = %+v, %v; want it unchanged", g, err)
	}

	// Telegram numbers posts per channel, so another channel may reuse message 42.
	if err := store.SaveDraft(
		ctx,
		&giveaway.Giveaway{ChatID: channelID - 1, MessageID: 42, CreatedBy: fanID},
	); err != nil {
		t.Errorf("SaveDraft in another channel: %v", err)
	}
}

func TestDeleteDraftKeepsPublished(t *testing.T) {
	store, _ := newStore(t)
	ctx := t.Context()

	publish(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})

	if err := store.SaveDraft(
		ctx,
		&giveaway.Giveaway{ChatID: channelID, MessageID: 43, CreatedBy: adminID},
	); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteDraft(ctx, adminID); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Draft(ctx, adminID); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("Draft after delete: err = %v, want ErrNotFound", err)
	}

	if _, err := store.Find(ctx, channelID, 42); err != nil {
		t.Errorf("published giveaway after DeleteDraft: %v", err)
	}

	if err := store.DeleteDraft(ctx, fanID); err != nil {
		t.Errorf("DeleteDraft without a draft: %v", err)
	}
}

func TestStoreJoin(t *testing.T) {
	store, conn := newStore(t)
	ctx := t.Context()

	g := publish(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})

	for i, want := range []bool{true, false} {
		added, err := store.Join(ctx, g.ID, fanID)
		if err != nil || added != want {
			t.Errorf("Join #%d = %v, %v; want %v, nil", i+1, added, err, want)
		}
	}

	var count int64
	if err := conn.Model(&giveaway.Participant{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Errorf("participants = %d, want 1", count)
	}

	if _, err := store.Join(ctx, g.ID, 7_000_000_099); err == nil {
		t.Error("Join for a user with no users row: err = nil, want a foreign key error")
	}
}
