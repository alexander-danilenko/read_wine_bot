// Package giveaway runs giveaways on channel posts: a channel admin starts one with /giveaway
// and a forwarded post, the bot attaches a join button to the post, and every tap records a
// participant.
package giveaway

import (
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// Giveaway is one row of the giveaways table: a channel post that carries the join button.
type Giveaway struct {
	ID        uint  `gorm:"primaryKey"`
	ChatID    int64 `gorm:"not null;uniqueIndex:idx_giveaways_post"` // the channel
	MessageID int64 `gorm:"not null;uniqueIndex:idx_giveaways_post"` // the post, numbered per channel
	CreatedBy int64 `gorm:"not null"`                                // Telegram ID of the admin who started it
	// Creator only declares the foreign key from created_by to users; it is never loaded.
	Creator   user.User `gorm:"foreignKey:CreatedBy;references:TelegramID"`
	Language  string    `gorm:"size:16;not null"` // locale of the button and the popups
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Participant is one row of the giveaway_participants table: a user who tapped the button.
type Participant struct {
	GiveawayID uint     `gorm:"primaryKey;autoIncrement:false"`
	Giveaway   Giveaway // declares the foreign key from giveaway_id to giveaways; never loaded
	// UserID (column user_id) holds the participant's users.telegram_id. It must not be named
	// TelegramID: with the same name, GORM reads the link backwards and puts the foreign key
	// on the users table.
	UserID    int64     `gorm:"primaryKey;autoIncrement:false"`
	User      user.User `gorm:"foreignKey:UserID;references:TelegramID"` // declares the foreign key; never loaded
	CreatedAt time.Time
}

// TableName names the participants table after the giveaways it belongs to.
func (Participant) TableName() string { return "giveaway_participants" }
