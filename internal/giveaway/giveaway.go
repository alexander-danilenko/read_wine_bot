// Package giveaway runs giveaways on channel posts. A channel admin creates one through the
// giveaway setup in the bot's private chat: forward the post, enter the end date and time,
// the winner count and the button text, and confirm a summary. Only then does the bot attach
// the join button to the post, and every tap records a participant.
package giveaway

import (
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// Giveaway is one row of the giveaways table: a channel post that carries, or during the
// setup will carry, the join button. An unpublished row without a cancel mark is the admin's
// unfinished setup; one with the mark is a cancelled giveaway.
type Giveaway struct {
	ID        uint  `gorm:"primaryKey;comment:Giveaway ID, used in the bot's buttons and commands"`
	ChatID    int64 `gorm:"not null;uniqueIndex:idx_giveaways_post;comment:Telegram ID of the channel with the post"`
	MessageID int64 `gorm:"not null;uniqueIndex:idx_giveaways_post;comment:Telegram message ID of the post, numbered per channel"`
	// The partial unique index allows one setup draft per admin.
	CreatedBy int64 `gorm:"not null;uniqueIndex:idx_giveaways_draft,where:published = false AND cancelled_at IS NULL;comment:Telegram user ID (users.telegram_id) of the admin who set the giveaway up"`
	// Creator only declares the foreign key from created_by to users; it is never loaded.
	Creator  user.User `gorm:"foreignKey:CreatedBy;references:TelegramID"`
	Language string    `gorm:"size:16;not null;comment:Locale code of the join button popups and the results message"`
	// Columns added to a table with rows need a default or NULL; giveaways created
	// before the setup steps have no end time and an empty button text.
	EndsAt     *time.Time `gorm:"comment:When entry ends and winners are drawn, NULL for no end time"`
	Winners    int        `gorm:"not null;default:1;check:chk_giveaways_winners,winners >= 1;comment:Number of winners to draw"`
	ButtonText string     `gorm:"not null;default:'';comment:Label of the join button under the post"`
	// FinishedAt is set together with the participants' winner marks; nil until then.
	FinishedAt *time.Time `gorm:"check:chk_giveaways_finished,finished_at IS NULL OR (published AND cancelled_at IS NULL);comment:When winners were drawn and saved, NULL until then"`
	// CancelledAt is set when the creator takes a published giveaway off its post: the row is
	// unpublished again but keeps its participants, so it is no setup draft. Publishing it
	// clears the mark. Nil for drafts and published giveaways.
	CancelledAt *time.Time `gorm:"check:chk_giveaways_cancelled,cancelled_at IS NULL OR NOT published;comment:When the creator cancelled the giveaway, NULL unless cancelled. Unpublished without this mark means an unfinished setup"`
	// Published is set once the join button is on the post. The default is true so that
	// giveaways older than the setup, which all have their button, count as published.
	// GORM leaves a false zero value out of an INSERT for a column with a default, so
	// Store.SaveDraft names the column explicitly.
	Published bool      `gorm:"not null;default:true;comment:Whether the join button is on the post. False for unfinished setups and cancelled giveaways"`
	CreatedAt time.Time `gorm:"comment:When the setup started"`
	UpdatedAt time.Time `gorm:"comment:When the row last changed"`
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
	GiveawayID uint     `gorm:"primaryKey;autoIncrement:false;comment:Giveaway (giveaways.id) the user joined"`
	Giveaway   Giveaway // declares the foreign key from giveaway_id to giveaways; never loaded
	// UserID (column user_id) holds the participant's users.telegram_id. It must not be named
	// TelegramID: with the same name, GORM reads the link backwards and puts the foreign key
	// on the users table.
	UserID    int64     `gorm:"primaryKey;autoIncrement:false;comment:Telegram user ID (users.telegram_id) of the participant"`
	User      user.User `gorm:"foreignKey:UserID;references:TelegramID"` // declares the foreign key; never loaded
	Winner    bool      `gorm:"not null;default:false;comment:Whether the draw picked this participant as a winner"`
	CreatedAt time.Time `gorm:"comment:When the user tapped the join button"`
}

// TableName names the participants table after the giveaways it belongs to.
func (Participant) TableName() string { return "giveaway_participants" }
