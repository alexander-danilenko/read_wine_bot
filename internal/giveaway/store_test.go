package giveaway_test

import (
	"errors"
	"path/filepath"
	"testing"

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

func TestCreateAndFind(t *testing.T) {
	store, _ := newStore(t)
	ctx := t.Context()

	created := &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID, Language: "uk"}
	if err := store.Create(ctx, created); err != nil {
		t.Fatal(err)
	}

	got, err := store.Find(ctx, channelID, 42)
	if err != nil {
		t.Fatal(err)
	}

	if got.ID != created.ID || got.CreatedBy != adminID || got.Language != "uk" {
		t.Errorf("Find = %+v, want ID %d, author %d, language uk", got, created.ID, adminID)
	}

	if _, err := store.Find(ctx, channelID, 43); !errors.Is(err, giveaway.ErrNotFound) {
		t.Errorf("Find unknown post: err = %v, want ErrNotFound", err)
	}
}

func TestCreateDuplicate(t *testing.T) {
	store, _ := newStore(t)
	ctx := t.Context()

	if err := store.Create(ctx, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID}); err != nil {
		t.Fatal(err)
	}

	err := store.Create(ctx, &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID})
	if !errors.Is(err, giveaway.ErrExists) {
		t.Errorf("second Create for the same post: err = %v, want ErrExists", err)
	}

	// Telegram numbers posts per channel, so another channel may reuse message 42.
	if err := store.Create(
		ctx,
		&giveaway.Giveaway{ChatID: channelID - 1, MessageID: 42, CreatedBy: adminID},
	); err != nil {
		t.Errorf("Create in another channel: %v", err)
	}
}

func TestJoin(t *testing.T) {
	store, conn := newStore(t)
	ctx := t.Context()

	g := &giveaway.Giveaway{ChatID: channelID, MessageID: 42, CreatedBy: adminID}
	if err := store.Create(ctx, g); err != nil {
		t.Fatal(err)
	}

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
