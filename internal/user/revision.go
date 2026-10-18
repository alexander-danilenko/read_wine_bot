package user

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Profile is what Telegram tells the bot about a user. A change to any field but TelegramID
// writes a new revision.
type Profile struct {
	TelegramID   int64
	IsBot        bool
	FirstName    string
	LastName     string
	Username     string
	LanguageCode string // "" when Telegram left it out; Record then keeps the saved one
	IsPremium    bool
}

// Revision is one row of the user_revisions table: a snapshot of a user's profile. Revisions
// are only ever added, never changed or deleted.
type Revision struct {
	ID uint `gorm:"primaryKey;comment:Row ID"`
	// UserID (column user_id) holds the user's users.telegram_id. It must not be named
	// TelegramID: with the same name, GORM reads the link backwards and puts the foreign key
	// on the users table.
	UserID       int64     `gorm:"not null;uniqueIndex:idx_user_revisions_number;comment:Telegram user ID (users.telegram_id) this snapshot belongs to"`
	User         User      `gorm:"foreignKey:UserID;references:TelegramID"` // declares the foreign key; never loaded
	Number       int       `gorm:"not null;uniqueIndex:idx_user_revisions_number;comment:Revision number, counting per user from 1"`
	IsBot        bool      `gorm:"not null;default:false;comment:Whether the account was a Telegram bot"`
	FirstName    string    `gorm:"not null;default:'';comment:First name at this revision"`
	LastName     string    `gorm:"not null;default:'';comment:Last name at this revision, empty when the user had none"`
	Username     string    `gorm:"not null;default:'';comment:Username without the @ at this revision, empty when the user had none"`
	LanguageCode string    `gorm:"not null;default:'';comment:Telegram app language at this revision (IETF tag)"`
	IsPremium    bool      `gorm:"not null;default:false;comment:Whether the user had Telegram Premium"`
	CreatedAt    time.Time `gorm:"comment:When the bot recorded this profile"`
}

// TableName names the table user_revisions; GORM would call it revisions.
func (Revision) TableName() string {
	return "user_revisions"
}

// keptLanguageCode is the language code to save from the argument: the saved one when the
// argument is empty, since Telegram leaves it out of some updates.
const keptLanguageCode = "COALESCE(NULLIF(?, ''), language_code)"

// changedProfile matches a users row whose profile differs from the one in the arguments,
// in order: is_bot, first_name, last_name, username, is_premium, language_code.
const changedProfile = "is_bot <> ? OR first_name <> ? OR last_name <> ? OR username <> ? OR is_premium <> ? " +
	"OR language_code <> " + keptLanguageCode

// Record saves the user's profile, creating the user on first use, and writes a new revision
// when the profile differs from the saved one. It leaves the saved language and pending
// command as they are.
func (s *Store) Record(ctx context.Context, p Profile) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Every branch starts with a write, so concurrent updates of one user queue on its row
		// lock instead of both reading the same revision number.
		u := User{
			TelegramID: p.TelegramID, IsBot: p.IsBot, FirstName: p.FirstName, LastName: p.LastName,
			Username: p.Username, LanguageCode: p.LanguageCode, IsPremium: p.IsPremium, Revision: 1,
		}

		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&u)
		if res.Error != nil {
			return res.Error
		}

		if res.RowsAffected == 0 {
			res = tx.Model(&User{}).
				Where("telegram_id = ?", p.TelegramID).
				Where(changedProfile, p.IsBot, p.FirstName, p.LastName, p.Username, p.IsPremium, p.LanguageCode).
				Updates(map[string]any{
					"is_bot":        p.IsBot,
					"first_name":    p.FirstName,
					"last_name":     p.LastName,
					"username":      p.Username,
					"is_premium":    p.IsPremium,
					"language_code": gorm.Expr(keptLanguageCode, p.LanguageCode),
					"revision":      gorm.Expr("revision + 1"),
				})
			if res.Error != nil || res.RowsAffected == 0 {
				return res.Error
			}

			// The snapshot comes from the saved row, which holds any language code kept above.
			if err := tx.Take(&u, p.TelegramID).Error; err != nil {
				return err
			}
		}

		return tx.Create(&Revision{
			UserID: u.TelegramID, Number: u.Revision, IsBot: u.IsBot, FirstName: u.FirstName,
			LastName: u.LastName, Username: u.Username, LanguageCode: u.LanguageCode, IsPremium: u.IsPremium,
		}).Error
	})
	if err != nil {
		return fmt.Errorf("record user %d: %w", p.TelegramID, err)
	}

	return nil
}
