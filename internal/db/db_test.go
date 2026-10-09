package db_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	for _, table := range []any{&user.User{}, &giveaway.Giveaway{}, &giveaway.Participant{}, &giveaway.Draft{}} {
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
		"giveaway_drafts":       {"FOREIGN KEY (`user_id`) REFERENCES `users`"},
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

// oldGiveaway and oldParticipant are the giveaway tables as the bot created them before the
// setup steps added end times, winners and drafts.
type oldGiveaway struct {
	ID        uint      `gorm:"primaryKey"`
	ChatID    int64     `gorm:"not null;uniqueIndex:idx_giveaways_post"`
	MessageID int64     `gorm:"not null;uniqueIndex:idx_giveaways_post"`
	CreatedBy int64     `gorm:"not null"`
	Creator   user.User `gorm:"foreignKey:CreatedBy;references:TelegramID"`
	Language  string    `gorm:"size:16;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (oldGiveaway) TableName() string { return "giveaways" }

type oldParticipant struct {
	GiveawayID uint        `gorm:"primaryKey;autoIncrement:false"`
	Giveaway   oldGiveaway `gorm:"foreignKey:GiveawayID"`
	UserID     int64       `gorm:"primaryKey;autoIncrement:false"`
	User       user.User   `gorm:"foreignKey:UserID;references:TelegramID"`
	CreatedAt  time.Time
}

func (oldParticipant) TableName() string { return "giveaway_participants" }

// createOldDatabase creates the database file at path with the pre-setup giveaway tables and
// one giveaway with a participant.
func createOldDatabase(t *testing.T, path string) {
	t.Helper()

	old, err := gorm.Open(sqlite.Open(path+"?_pragma=foreign_keys(1)"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}

	if err := old.AutoMigrate(&user.User{}, &oldGiveaway{}, &oldParticipant{}); err != nil {
		t.Fatal(err)
	}

	for _, stmt := range []string{
		"INSERT INTO users (telegram_id, language) VALUES (1, 'uk')",
		"INSERT INTO giveaways (id, chat_id, message_id, created_by, language) VALUES (5, -100, 42, 1, 'uk')",
		"INSERT INTO giveaway_participants (giveaway_id, user_id) VALUES (5, 1)",
	} {
		if err := old.Exec(stmt).Error; err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	if sqlDB, dbErr := old.DB(); dbErr == nil {
		_ = sqlDB.Close()
	}
}

// TestOpenExistingGiveaways opens a database whose giveaway tables predate the setup steps
// and already hold a giveaway with a participant, as the deployed bot's database does.
func TestOpenExistingGiveaways(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bot.db")
	createOldDatabase(t, path)

	conn := open(t, path)

	var g giveaway.Giveaway
	if err := conn.First(&g, 5).Error; err != nil {
		t.Fatal(err)
	}

	if g.Winners != 1 || g.ButtonText != "" || g.EndsAt != nil || g.FinishedAt != nil {
		t.Errorf("existing giveaway = %+v; want 1 winner, empty button text, no end or finish time", g)
	}

	var p giveaway.Participant
	if err := conn.First(&p, "giveaway_id = ? AND user_id = ?", 5, 1).Error; err != nil {
		t.Fatal(err)
	}

	if p.Winner {
		t.Error("existing participant is marked as a winner")
	}
}

func TestOpenMissingDir(t *testing.T) {
	if _, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "missing", "bot.db")); err == nil {
		t.Fatal("err = nil, want an error for a directory that does not exist")
	}
}
