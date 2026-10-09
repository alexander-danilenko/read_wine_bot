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
