package db_test

import (
	"path/filepath"
	"testing"

	"github.com/alexander-danilenko/read_wine_bot/internal/db"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

func TestOpen(t *testing.T) {
	conn, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "bot.db"))
	if err != nil {
		t.Fatal(err)
	}

	sqlDB, err := conn.DB()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = sqlDB.Close() })

	var foreignKeys int
	if err = conn.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatal(err)
	}

	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}

	if !conn.Migrator().HasTable(&user.User{}) {
		t.Error("users table missing after Open")
	}
}

func TestOpenMissingDir(t *testing.T) {
	if _, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "missing", "bot.db")); err == nil {
		t.Fatal("err = nil, want an error for a directory that does not exist")
	}
}
