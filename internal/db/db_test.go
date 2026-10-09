package db_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexander-danilenko/read_wine_bot/internal/config"
	"github.com/alexander-danilenko/read_wine_bot/internal/db"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

func TestOpenSQLite(t *testing.T) {
	conn, err := db.Open(t.Context(), config.DriverSQLite, filepath.Join(t.TempDir(), "bot.db"))
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

func TestOpenUnknownDriver(t *testing.T) {
	if _, err := db.Open(t.Context(), "mysql", "x"); !errors.Is(err, db.ErrUnknownDriver) {
		t.Fatalf("err = %v, want %v", err, db.ErrUnknownDriver)
	}
}

func TestOpenErrorHidesDSN(t *testing.T) {
	const password = "s3cretPW"

	// The typo in sslmode makes pgx fail to parse the DSN, and its parse error quotes the DSN.
	dsns := []string{
		"postgres://bot@host/db?password=" + password + "&sslmode=requre",
		"postgres://bot:" + password + "@host/db?sslmode=requre",
		"host=db user=bot password=" + password + " sslmode=requre",
	}

	for _, dsn := range dsns {
		_, err := db.Open(t.Context(), config.DriverPostgres, dsn)
		if err == nil {
			t.Fatalf("Open(%q): err = nil, want a parse error", dsn)
		}

		if strings.Contains(err.Error(), password) {
			t.Errorf("Open error leaks the password: %v", err)
		}
	}
}
