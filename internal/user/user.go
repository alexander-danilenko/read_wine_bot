// Package user stores what the bot remembers about each Telegram user.
package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// User is one row of the users table.
type User struct {
	TelegramID int64  `gorm:"primaryKey;autoIncrement:false"`
	FirstName  string `gorm:"not null;default:''"` // from Telegram, refreshed on every interaction
	LastName   string `gorm:"not null;default:''"` // "" when the user has none
	Language   string `gorm:"size:16;not null"`    // a locale code from internal/i18n, set with /language
	// PendingCommand is the command waiting for the user's next message: "giveaway" after
	// /giveaway, "" otherwise. Columns added to a table with rows need a default in SQLite.
	PendingCommand string `gorm:"size:32;not null;default:''"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Store reads and writes users.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store backed by db. The users table must already exist.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Language returns the language the user chose with /language, or "" if they never chose one.
func (s *Store) Language(ctx context.Context, telegramID int64) (string, error) {
	u, err := s.read(ctx, telegramID, "language")
	if err != nil {
		return "", fmt.Errorf("read language of user %d: %w", telegramID, err)
	}

	return u.Language, nil
}

// SetLanguage saves the user's language, creating the user on first use.
func (s *Store) SetLanguage(ctx context.Context, telegramID int64, lang string) error {
	if err := s.upsert(ctx, &User{TelegramID: telegramID, Language: lang}, "language"); err != nil {
		return fmt.Errorf("save language of user %d: %w", telegramID, err)
	}

	return nil
}

// SaveNames saves the user's first and last name, creating the user on first use. It leaves
// the saved language and pending command as they are.
func (s *Store) SaveNames(ctx context.Context, telegramID int64, firstName, lastName string) error {
	u := &User{TelegramID: telegramID, FirstName: firstName, LastName: lastName}
	if err := s.upsert(ctx, u, "first_name", "last_name"); err != nil {
		return fmt.Errorf("save names of user %d: %w", telegramID, err)
	}

	return nil
}

// PendingCommand returns the command waiting for the user's next message, or "" if none.
func (s *Store) PendingCommand(ctx context.Context, telegramID int64) (string, error) {
	u, err := s.read(ctx, telegramID, "pending_command")
	if err != nil {
		return "", fmt.Errorf("read pending command of user %d: %w", telegramID, err)
	}

	return u.PendingCommand, nil
}

// SetPendingCommand saves the command waiting for the user's next message; "" clears it.
func (s *Store) SetPendingCommand(ctx context.Context, telegramID int64, command string) error {
	if err := s.upsert(ctx, &User{TelegramID: telegramID, PendingCommand: command}, "pending_command"); err != nil {
		return fmt.Errorf("save pending command of user %d: %w", telegramID, err)
	}

	return nil
}

// read returns the user's row with only column filled, or a zero User if the user is unknown.
func (s *Store) read(ctx context.Context, telegramID int64, column string) (User, error) {
	var u User

	err := s.db.WithContext(ctx).Select(column).Take(&u, telegramID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, nil
	}

	return u, err
}

// upsert inserts u, or, if the user exists, updates only columns and updated_at.
func (s *Store) upsert(ctx context.Context, u *User, columns ...string) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "telegram_id"}},
		DoUpdates: clause.AssignmentColumns(append(columns, "updated_at")),
	}).Create(u).Error
}
