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

// cancelled saves g, publishes it, adds the fan as a participant and cancels it.
func cancelled(t *testing.T, store *giveaway.Store, g *giveaway.Giveaway) giveaway.Giveaway {
	t.Helper()

	p := publish(t, store, g)

	if _, err := store.Join(t.Context(), p.ID, fanID); err != nil {
		t.Fatal(err)
	}

	if err := store.Cancel(t.Context(), p.ID); err != nil {
		t.Fatal(err)
	}

	c, err := store.ByID(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func TestDeleteDraftKeepsCancelled(t *testing.T) {
	store, conn := newStore(t)
	ctx := t.Context()

	g := cancelled(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})

	if err := store.SaveDraft(
		ctx,
		&giveaway.Giveaway{ChatID: channelID, MessageID: 43, CreatedBy: adminID},
	); err != nil {
		t.Fatal(err)
	}

	if d, err := store.Draft(ctx, adminID); err != nil || d.MessageID != 43 {
		t.Errorf("Draft = %+v, %v; want the draft on post 43", d, err)
	}

	if err := store.DeleteDraft(ctx, adminID); err != nil {
		t.Fatalf("DeleteDraft next to a cancelled giveaway with a participant: %v", err)
	}

	var participants int64
	if err := conn.Model(&giveaway.Participant{}).
		Where("giveaway_id = ?", g.ID).
		Count(&participants).
		Error; err != nil {
		t.Fatal(err)
	}

	if participants != 1 {
		t.Errorf("participants of the cancelled giveaway = %d, want 1", participants)
	}
}

func TestSaveDraftOnCancelledPost(t *testing.T) {
	store, _ := newStore(t)
	ctx := t.Context()

	g := cancelled(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})

	if _, err := store.Draft(ctx, adminID); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("Draft with only a cancelled giveaway: err = %v, want ErrNotFound", err)
	}

	// Another admin forwarding the cancelled post cannot take it over.
	err := store.SaveDraft(ctx, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: fanID, Winners: 5})
	if !errors.Is(err, giveaway.ErrExists) {
		t.Errorf("SaveDraft on a cancelled post: err = %v, want ErrExists", err)
	}

	if got, err := store.ByID(ctx, g.ID); err != nil || got.CreatedBy != adminID || got.Winners == 5 {
		t.Errorf("cancelled giveaway = %+v, %v; want it unchanged", got, err)
	}
}

func TestLookupSeesPublishedAndCancelled(t *testing.T) {
	store, _ := newStore(t)
	ctx := t.Context()

	published := publish(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 41, CreatedBy: adminID})
	gone := cancelled(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})

	if err := store.SaveDraft(ctx, &giveaway.Giveaway{ChatID: channelID, MessageID: 43, CreatedBy: fanID}); err != nil {
		t.Fatal(err)
	}

	draft, err := store.Draft(ctx, fanID)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		message int64
		id      uint
		want    uint // 0 for ErrNotFound
	}{
		{name: "published", message: 41, id: published.ID, want: published.ID},
		{name: "cancelled", message: 42, id: gone.ID, want: gone.ID},
		{name: "draft", message: 43, id: draft.ID},
		{name: "missing", message: 44, id: 999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for lookup, get := range map[string]func() (giveaway.Giveaway, error){
				"Post": func() (giveaway.Giveaway, error) { return store.Post(ctx, channelID, tt.message) },
				"ByID": func() (giveaway.Giveaway, error) { return store.ByID(ctx, tt.id) },
			} {
				got, err := get()
				if tt.want == 0 {
					if !errors.Is(err, giveaway.ErrNotFound) {
						t.Errorf("%s: err = %v, want ErrNotFound", lookup, err)
					}

					continue
				}

				if err != nil || got.ID != tt.want {
					t.Errorf("%s = %d, %v; want %d", lookup, got.ID, err, tt.want)
				}
			}
		})
	}
}

func TestCancel(t *testing.T) {
	store, _ := newStore(t)
	ctx := t.Context()

	g := cancelled(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})
	if g.Published || g.CancelledAt == nil {
		t.Errorf("after Cancel = %+v; want unpublished with a cancel time", g)
	}

	if err := store.Cancel(ctx, g.ID); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("second Cancel: err = %v, want ErrNotFound", err)
	}
}

func TestRestore(t *testing.T) {
	store, _ := newStore(t)
	ctx := t.Context()

	g := cancelled(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})

	if err := store.Publish(ctx, g.ID); err != nil {
		t.Fatal(err)
	}

	if got, err := store.ByID(ctx, g.ID); err != nil || !got.Published || got.CancelledAt != nil {
		t.Errorf("after Publish = %+v, %v; want published with no cancel time", got, err)
	}

	if added, err := store.Join(ctx, g.ID, fanID); err != nil || added {
		t.Errorf("Join after restore = %v, %v; want the earlier participant kept", added, err)
	}
}

func TestFinishedGiveawayIsLocked(t *testing.T) {
	store, conn := newStore(t)
	ctx := t.Context()

	g := publish(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, Winners: 1})
	if err := conn.Model(&giveaway.Giveaway{}).
		Where("id = ?", g.ID).
		Update("finished_at", time.Now()).
		Error; err != nil {
		t.Fatal(err)
	}

	if err := store.Cancel(ctx, g.ID); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("Cancel of a finished giveaway: err = %v, want ErrNotFound", err)
	}

	if err := store.SaveFields(ctx, g.ID, nil, 3, "Go"); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("SaveFields of a finished giveaway: err = %v, want ErrNotFound", err)
	}

	if got, err := store.ByID(ctx, g.ID); err != nil || got.Winners != 1 || !got.Published {
		t.Errorf("finished giveaway = %+v, %v; want it unchanged", got, err)
	}
}

func TestSaveFields(t *testing.T) {
	store, _ := newStore(t)
	ctx := t.Context()
	ends := time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC)

	for _, g := range []giveaway.Giveaway{
		publish(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 41, CreatedBy: adminID}),
		cancelled(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID}),
	} {
		if err := store.SaveFields(ctx, g.ID, &ends, 3, "Беру участь"); err != nil {
			t.Fatal(err)
		}

		got, err := store.ByID(ctx, g.ID)
		if err != nil || got.EndsAt == nil || !got.EndsAt.Equal(ends) || got.Winners != 3 ||
			got.ButtonText != "Беру участь" || got.Published != g.Published {
			t.Errorf("after SaveFields = %+v, %v; want the new values and the same published state", got, err)
		}
	}
}

func TestSaveFieldsOfDraft(t *testing.T) {
	store, _ := newStore(t)
	ctx := t.Context()

	if err := store.SaveDraft(
		ctx,
		&giveaway.Giveaway{ChatID: channelID, MessageID: 43, CreatedBy: adminID},
	); err != nil {
		t.Fatal(err)
	}

	d, err := store.Draft(ctx, adminID)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SaveFields(ctx, d.ID, nil, 3, "Go"); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("SaveFields of a draft: err = %v, want ErrNotFound", err)
	}
}

func TestRunning(t *testing.T) {
	store, conn := newStore(t)
	ctx := t.Context()
	at := func(day int) *time.Time {
		t := time.Date(2026, 10, day, 16, 0, 0, 0, time.UTC)

		return &t
	}

	later := publish(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 41, CreatedBy: adminID, EndsAt: at(25)})
	sooner := publish(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, EndsAt: at(1)})
	cancelled(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 43, CreatedBy: adminID, EndsAt: at(20)})

	finished := publish(
		t,
		store,
		&giveaway.Giveaway{ChatID: channelID, MessageID: 44, CreatedBy: adminID, EndsAt: at(2)},
	)
	if err := conn.Model(&giveaway.Giveaway{}).
		Where("id = ?", finished.ID).
		Update("finished_at", time.Now()).
		Error; err != nil {
		t.Fatal(err)
	}

	publish(t, store, &giveaway.Giveaway{ChatID: channelID, MessageID: 45, CreatedBy: fanID, EndsAt: at(3)})

	if err := store.SaveDraft(
		ctx,
		&giveaway.Giveaway{ChatID: channelID, MessageID: 46, CreatedBy: adminID},
	); err != nil {
		t.Fatal(err)
	}

	got, err := store.Running(ctx, adminID)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 || got[0].ID != sooner.ID || got[1].ID != later.ID {
		t.Errorf("Running = %+v; want the giveaways on posts 42 and 41, in that order", got)
	}
}
