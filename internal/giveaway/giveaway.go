// Package giveaway runs giveaways on channel posts: a channel admin starts one from the menu
// with a forwarded post, the bot attaches a join button to the post, and every tap records a
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
	Creator  user.User `gorm:"foreignKey:CreatedBy;references:TelegramID"`
	Language string    `gorm:"size:16;not null"` // locale of the button and the popups
	// Columns added to a table with rows need a default or NULL in SQLite; giveaways created
	// before the setup steps have no end time and an empty button text.
	EndsAt     *time.Time // when entry ends, in UTC; nil for giveaways older than the setup steps
	Winners    int        `gorm:"not null;default:1"`
	ButtonText string     `gorm:"not null;default:''"`
	// FinishedAt is set together with the participants' winner marks; nil until then.
	FinishedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// State is where a giveaway is in its life.
type State int

// Giveaway states.
const (
	Active   State = iota // taking participants
	Due                   // past its end time, winners not recorded yet
	Finished              // winners recorded
)

// State returns the giveaway's state at now. A giveaway without an end time stays active
// until it is finished.
func (g Giveaway) State(now time.Time) State {
	switch {
	case g.FinishedAt != nil:
		return Finished
	case g.EndsAt != nil && !now.Before(*g.EndsAt):
		return Due
	default:
		return Active
	}
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
	Winner    bool      `gorm:"not null;default:false"`
	CreatedAt time.Time
}

// TableName names the participants table after the giveaways it belongs to.
func (Participant) TableName() string { return "giveaway_participants" }

// Draft is one row of the giveaway_drafts table: the inputs of a user's unfinished giveaway
// setup. A user has at most one; a new setup overwrites it, and Confirm or Cancel deletes it.
type Draft struct {
	UserID       int64     `gorm:"primaryKey;autoIncrement:false"`          // users.telegram_id of the admin
	User         user.User `gorm:"foreignKey:UserID;references:TelegramID"` // declares the foreign key; never loaded
	ChatID       int64     // the channel of the forwarded post
	MessageID    int64     // the forwarded post
	ChannelTitle string    // shown in the summary
	EndDate      string    // YYYY-MM-DD in Kyiv time, as typed
	EndsAt       time.Time // end date and time in UTC, set at the end time step
	Winners      int
	ButtonText   string
	UpdatedAt    time.Time
}

// TableName names the drafts table after the giveaways it prepares.
func (Draft) TableName() string { return "giveaway_drafts" }
