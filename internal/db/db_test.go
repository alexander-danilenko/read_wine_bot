package db_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/alexander-danilenko/read_wine_bot/internal/db"
	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// open opens the database file at path with db.Open and closes it when the test ends.
func open(t *testing.T, path string) *gorm.DB {
	t.Helper()

	conn, err := db.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	sqlDB, err := conn.DB()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = sqlDB.Close() })

	return conn
}

func TestOpen(t *testing.T) {
	conn := open(t, filepath.Join(t.TempDir(), "bot.db"))

	var foreignKeys int
	if err := conn.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatal(err)
	}

	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}

	for _, table := range []any{&user.User{}, &giveaway.Giveaway{}, &giveaway.Participant{}} {
		if !conn.Migrator().HasTable(table) {
			t.Errorf("table for %T missing after Open", table)
		}
	}
}

// TestOpenForeignKeys checks that the giveaway tables point at users, not the other way
// round: a foreign key on users would make every insert into users fail.
func TestOpenForeignKeys(t *testing.T) {
	conn := open(t, filepath.Join(t.TempDir(), "bot.db"))

	if err := user.NewStore(conn).SaveNames(t.Context(), 1, "Olena", ""); err != nil {
		t.Errorf("insert into users after Open: %v", err)
	}

	for table, want := range map[string][]string{
		"users":                 nil,
		"giveaways":             {"REFERENCES `users`"},
		"giveaway_participants": {"FOREIGN KEY (`user_id`) REFERENCES `users`", "REFERENCES `giveaways`"},
	} {
		var ddl string
		if err := conn.Raw("SELECT sql FROM sqlite_master WHERE name = ?", table).Scan(&ddl).Error; err != nil {
			t.Fatal(err)
		}

		if got := strings.Count(ddl, "REFERENCES"); got != len(want) {
			t.Errorf("%s has %d foreign keys, want %d: %s", table, got, len(want), ddl)
		}

		for _, ref := range want {
			if !strings.Contains(ddl, ref) {
				t.Errorf("%s lacks %s: %s", table, ref, ddl)
			}
		}
	}
}

// TestOpenExistingUsers opens a database whose users table predates the name and pending
// command columns and already has a row, as the deployed bot's database does.
func TestOpenExistingUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bot.db")

	old, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}

	err = old.Exec("CREATE TABLE users (telegram_id integer PRIMARY KEY, language text NOT NULL, " +
		"created_at datetime, updated_at datetime); INSERT INTO users VALUES (1, 'uk', NULL, NULL)").Error
	if err != nil {
		t.Fatal(err)
	}

	if sqlDB, dbErr := old.DB(); dbErr == nil {
		_ = sqlDB.Close()
	}

	conn := open(t, path)

	var got user.User
	if err := conn.First(&got, 1).Error; err != nil {
		t.Fatal(err)
	}

	if got.Language != "uk" || got.FirstName != "" || got.PendingCommand != "" {
		t.Errorf("existing user = %+v; want language uk, empty names and pending command", got)
	}
}

func TestOpenMissingDir(t *testing.T) {
	if _, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "missing", "bot.db")); err == nil {
		t.Fatal("err = nil, want an error for a directory that does not exist")
	}
}
