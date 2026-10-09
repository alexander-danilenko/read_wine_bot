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
	Language   string `gorm:"size:16;not null"` // a locale code from internal/i18n, set with /language
	CreatedAt  time.Time
	UpdatedAt  time.Time
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
	var u User

	err := s.db.WithContext(ctx).Select("language").Take(&u, telegramID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("read language of user %d: %w", telegramID, err)
	}

	return u.Language, nil
}

// SetLanguage saves the user's language, creating the user on first use.
func (s *Store) SetLanguage(ctx context.Context, telegramID int64, lang string) error {
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "telegram_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"language", "updated_at"}),
	}).Create(&User{TelegramID: telegramID, Language: lang}).Error
	if err != nil {
		return fmt.Errorf("save language of user %d: %w", telegramID, err)
	}

	return nil
}
