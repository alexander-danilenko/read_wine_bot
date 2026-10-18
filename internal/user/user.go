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

// User is one row of the users table: the user's latest profile plus the bot's own state.
// Columns added to a table with rows need a default. Every column carries a comment: tag,
// which AutoMigrate stores as the column's description in the database.
type User struct {
	TelegramID int64  `gorm:"primaryKey;autoIncrement:false;comment:Telegram user ID"`
	IsBot      bool   `gorm:"not null;default:false;comment:Whether the account is a Telegram bot"`
	FirstName  string `gorm:"not null;default:'';comment:First name from Telegram, refreshed on every interaction"`
	LastName   string `gorm:"not null;default:'';comment:Last name from Telegram, empty when the user has none"`
	Username   string `gorm:"not null;default:'';comment:Telegram username without the @, empty when the user has none"`
	// LanguageCode is the Telegram app's language as Telegram reports it, not the bot's menu
	// choice, which is Language.
	LanguageCode string `gorm:"not null;default:'';comment:Language of the user's Telegram app as Telegram reports it (IETF tag), not the bot language"`
	IsPremium    bool   `gorm:"not null;default:false;comment:Whether the user has Telegram Premium"`
	Revision     int    `gorm:"not null;default:0;comment:Number of the latest row in user_revisions for this user, 0 before the first"`
	Language     string `gorm:"size:16;not null;comment:Bot language the user chose from the menu (locale code from internal/i18n), empty if never chosen"`
	// PendingCommand is what the bot waits for in the user's next message, such as a giveaway
	// setup step, or "" for nothing.
	PendingCommand string `gorm:"size:32;not null;default:'';comment:What the bot waits for in the user's next private message, such as a giveaway setup step, empty for nothing"`
	// KeyboardMessageID is the bot's latest message with buttons in the user's private chat,
	// which the bot deletes before it sends the next one.
	KeyboardMessageID int64     `gorm:"not null;default:0;comment:ID of the bot's latest message with buttons in the user's private chat, deleted before the bot sends the next one, 0 for none"`
	CreatedAt         time.Time `gorm:"comment:When the bot first saw the user"`
	UpdatedAt         time.Time `gorm:"comment:When the row last changed"`
}

// Store reads and writes users.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store backed by db. The users table must already exist.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Language returns the language the user chose from the menu, or "" if they never chose one.
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

// MovePendingCommand changes the user's pending command from to to, and reports whether
// it did; it does nothing unless the pending command equals from. It is one conditional
// update, so when several updates race for one step, exactly one of them moves it.
func (s *Store) MovePendingCommand(ctx context.Context, telegramID int64, from, to string) (bool, error) {
	res := s.db.WithContext(ctx).Model(&User{}).
		Where("telegram_id = ? AND pending_command = ?", telegramID, from).
		Update("pending_command", to)
	if res.Error != nil {
		return false, fmt.Errorf("move pending command of user %d: %w", telegramID, res.Error)
	}

	return res.RowsAffected == 1, nil
}

// PendingCommand returns the user's pending command, or "" if there is none.
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

// KeyboardMessage returns the ID of the bot's latest message with buttons in the user's
// private chat, or 0 if there is none.
func (s *Store) KeyboardMessage(ctx context.Context, telegramID int64) (int64, error) {
	u, err := s.read(ctx, telegramID, "keyboard_message_id")
	if err != nil {
		return 0, fmt.Errorf("read keyboard message of user %d: %w", telegramID, err)
	}

	return u.KeyboardMessageID, nil
}

// SetKeyboardMessage saves the bot's latest message with buttons in the user's private chat;
// 0 clears it.
func (s *Store) SetKeyboardMessage(ctx context.Context, telegramID, messageID int64) error {
	u := &User{TelegramID: telegramID, KeyboardMessageID: messageID}
	if err := s.upsert(ctx, u, "keyboard_message_id"); err != nil {
		return fmt.Errorf("save keyboard message of user %d: %w", telegramID, err)
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
