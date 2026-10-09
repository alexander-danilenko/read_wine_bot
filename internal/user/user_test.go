package user_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

func newStore(t *testing.T) (*user.Store, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(&user.User{}, &user.Revision{}); err != nil {
		t.Fatal(err)
	}

	return user.NewStore(db), db
}

func TestLanguage(t *testing.T) {
	const id int64 = 7_000_000_001 // larger than int32, as real Telegram IDs can be

	store, db := newStore(t)
	ctx := t.Context()

	got, err := store.Language(ctx, id)
	if err != nil || got != "" {
		t.Fatalf("unknown user: Language = %q, %v; want \"\", nil", got, err)
	}

	if err = store.SetLanguage(ctx, id, "uk"); err != nil {
		t.Fatal(err)
	}

	var first user.User
	if err = db.First(&first, id).Error; err != nil {
		t.Fatal(err)
	}

	if err = store.SetLanguage(ctx, id, "en"); err != nil {
		t.Fatal(err)
	}

	got, err = store.Language(ctx, id)
	if err != nil || got != "en" {
		t.Fatalf("after two saves: Language = %q, %v; want \"en\", nil", got, err)
	}

	var second user.User
	if err := db.First(&second, id).Error; err != nil {
		t.Fatal(err)
	}

	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want after %v", second.UpdatedAt, first.UpdatedAt)
	}
}

// readUser returns the user's full row.
func readUser(t *testing.T, db *gorm.DB, id int64) user.User {
	t.Helper()

	var u user.User
	if err := db.First(&u, id).Error; err != nil {
		t.Fatal(err)
	}

	return u
}

func TestSetLanguageKeepsNames(t *testing.T) {
	const id int64 = 7_000_000_002

	store, db := newStore(t)
	ctx := t.Context()

	if err := store.Record(ctx, user.Profile{TelegramID: id, FirstName: "Olena", LastName: "Koval"}); err != nil {
		t.Fatal(err)
	}

	if err := store.SetLanguage(ctx, id, "en"); err != nil {
		t.Fatal(err)
	}

	if got := readUser(t, db, id); got.FirstName != "Olena" || got.LastName != "Koval" {
		t.Errorf("SetLanguage changed names to %q %q", got.FirstName, got.LastName)
	}
}

func TestMovePendingCommand(t *testing.T) {
	const id int64 = 7_000_000_004

	store, db := newStore(t)
	ctx := t.Context()

	moved, err := store.MovePendingCommand(ctx, id, "giveaway:post", "giveaway:end_date")
	if err != nil || moved {
		t.Fatalf("unknown user: MovePendingCommand = %v, %v; want false, nil", moved, err)
	}

	if err = store.SetPendingCommand(ctx, id, "giveaway:end_date"); err != nil {
		t.Fatal(err)
	}

	if moved, err = store.MovePendingCommand(ctx, id, "giveaway:winners", ""); err != nil || moved {
		t.Errorf("move from a step the user is not at = %v, %v; want false, nil", moved, err)
	}

	// Two updates race for one step: only the first may move it.
	for i, want := range []bool{true, false} {
		moved, err := store.MovePendingCommand(ctx, id, "giveaway:end_date", "giveaway:end_time")
		if err != nil || moved != want {
			t.Errorf("move #%d = %v, %v; want %v, nil", i+1, moved, err, want)
		}
	}

	if got := readUser(t, db, id); got.PendingCommand != "giveaway:end_time" {
		t.Errorf("after move: pending command = %q, want giveaway:end_time", got.PendingCommand)
	}
}

func TestPendingCommand(t *testing.T) {
	const id int64 = 7_000_000_005

	store, _ := newStore(t)
	ctx := t.Context()

	if got, err := store.PendingCommand(ctx, id); err != nil || got != "" {
		t.Errorf("unknown user: PendingCommand = %q, %v; want \"\", nil", got, err)
	}

	if err := store.SetPendingCommand(ctx, id, "giveaway:winners"); err != nil {
		t.Fatal(err)
	}

	if got, err := store.PendingCommand(ctx, id); err != nil || got != "giveaway:winners" {
		t.Errorf("PendingCommand = %q, %v; want giveaway:winners, nil", got, err)
	}
}
