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

	if err := db.AutoMigrate(&user.User{}); err != nil {
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

func TestSaveNamesNewUser(t *testing.T) {
	const id int64 = 7_000_000_002

	store, db := newStore(t)

	if err := store.SaveNames(t.Context(), id, "Olena", ""); err != nil {
		t.Fatal(err)
	}

	want := user.User{TelegramID: id, FirstName: "Olena"}
	got := readUser(t, db, id)
	got.CreatedAt, got.UpdatedAt = want.CreatedAt, want.UpdatedAt

	if got != want {
		t.Errorf("new user = %+v, want %+v", got, want)
	}
}

func TestSaveNamesKeepsLanguageAndPending(t *testing.T) {
	const id int64 = 7_000_000_002

	store, db := newStore(t)
	ctx := t.Context()

	for _, err := range []error{
		store.SaveNames(ctx, id, "Olena", ""),
		store.SetLanguage(ctx, id, "uk"),
		store.SetPendingCommand(ctx, id, "giveaway"),
		store.SaveNames(ctx, id, "Olena", "Koval"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	got := readUser(t, db, id)
	if got.FirstName != "Olena" || got.LastName != "Koval" || got.Language != "uk" || got.PendingCommand != "giveaway" {
		t.Errorf("after rename = %+v; want Olena Koval, language uk, pending giveaway", got)
	}
}

func TestSetLanguageKeepsNames(t *testing.T) {
	const id int64 = 7_000_000_002

	store, db := newStore(t)
	ctx := t.Context()

	if err := store.SaveNames(ctx, id, "Olena", "Koval"); err != nil {
		t.Fatal(err)
	}

	if err := store.SetLanguage(ctx, id, "en"); err != nil {
		t.Fatal(err)
	}

	if got := readUser(t, db, id); got.FirstName != "Olena" || got.LastName != "Koval" {
		t.Errorf("SetLanguage changed names to %q %q", got.FirstName, got.LastName)
	}
}

func TestPendingCommand(t *testing.T) {
	const id int64 = 7_000_000_003

	store, _ := newStore(t)
	ctx := t.Context()

	got, err := store.PendingCommand(ctx, id)
	if err != nil || got != "" {
		t.Fatalf("unknown user: PendingCommand = %q, %v; want \"\", nil", got, err)
	}

	for _, want := range []string{"giveaway", ""} {
		if err := store.SetPendingCommand(ctx, id, want); err != nil {
			t.Fatal(err)
		}

		got, err := store.PendingCommand(ctx, id)
		if err != nil || got != want {
			t.Errorf("PendingCommand = %q, %v; want %q, nil", got, err, want)
		}
	}
}
