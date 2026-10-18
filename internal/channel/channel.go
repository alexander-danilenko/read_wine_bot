// Package channel keeps the log of the channel's audience: every subscribe, unsubscribe and
// removal, and the time of every channel post. Both tables only grow; the owner analyses them
// with the SQL in docs/channel-analytics.md.
package channel

import (
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// Kind is what happened to a subscriber.
type Kind string

// Membership event kinds. The chk_channel_member_events_kind constraint allows only these.
const (
	Joined  Kind = "joined"  // subscribed
	Left    Kind = "left"    // unsubscribed
	Removed Kind = "removed" // removed or banned by an admin
)

// Event is one row of the channel_member_events table: one subscriber change. Rows are only
// ever added. The unique index makes an update Telegram delivers twice write one row.
type Event struct {
	ID     uint  `gorm:"primaryKey;comment:Row ID, in the order the bot recorded the events"`
	ChatID int64 `gorm:"not null;uniqueIndex:idx_channel_member_events_once;comment:Telegram ID of the channel"`
	// UserID (column user_id) holds the subscriber's users.telegram_id. It must not be named
	// TelegramID: with the same name, GORM reads the link backwards and puts the foreign key
	// on the users table.
	UserID int64     `gorm:"not null;uniqueIndex:idx_channel_member_events_once;comment:Telegram user ID (users.telegram_id) of the subscriber"`
	User   user.User `gorm:"foreignKey:UserID;references:TelegramID"` // declares the foreign key; never loaded
	Kind   Kind      `gorm:"size:16;not null;uniqueIndex:idx_channel_member_events_once;check:chk_channel_member_events_kind,kind IN ('joined', 'left', 'removed');comment:What happened: joined (subscribed), left (unsubscribed) or removed (removed or banned by an admin)"`
	// The invite link is a snapshot of the join, so a later rename of the link does not
	// rewrite history.
	InviteLink     string    `gorm:"not null;default:'';comment:Invite link the subscriber joined through, empty for other kinds or a join without a link. Links created by other admins end in an ellipsis instead of their secret part"`
	InviteLinkName string    `gorm:"not null;default:'';comment:Name the invite link had at the join, empty when the link has none or for other kinds"`
	HappenedAt     time.Time `gorm:"not null;uniqueIndex:idx_channel_member_events_once;comment:When the change happened, as Telegram reports it (to the second)"`
}

// TableName names the table channel_member_events; GORM would call it events.
func (Event) TableName() string {
	return "channel_member_events"
}

// Post is one row of the channel_posts table: a post published in the channel. Rows are only
// ever added.
type Post struct {
	ChatID    int64     `gorm:"primaryKey;autoIncrement:false;comment:Telegram ID of the channel"`
	MessageID int64     `gorm:"primaryKey;autoIncrement:false;comment:Telegram message ID of the post, numbered per channel"`
	PostedAt  time.Time `gorm:"not null;comment:When the post was published, as Telegram reports it (to the second)"`
}

// TableName names the table channel_posts; GORM would call it posts.
func (Post) TableName() string {
	return "channel_posts"
}
