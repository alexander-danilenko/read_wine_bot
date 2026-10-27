package channel

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store writes the audience log. A row that is already there is left as it is, so a repeated
// update is not an error.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store backed by db. The tables must already exist.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// AddEvent saves e unless the same event is already saved. The subscriber must be in users.
func (s *Store) AddEvent(ctx context.Context, e Event) error {
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&e).Error; err != nil {
		return fmt.Errorf("add %s event of user %d in chat %d: %w", e.Kind, e.UserID, e.ChatID, err)
	}

	return nil
}

// AddPost saves p unless the post is already saved.
func (s *Store) AddPost(ctx context.Context, p Post) error {
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&p).Error; err != nil {
		return fmt.Errorf("add post %d in chat %d: %w", p.MessageID, p.ChatID, err)
	}

	return nil
}

// SetBotChannel saves the bot's status in channel c, unless a newer status is already saved:
// Telegram can deliver updates out of order.
func (s *Store) SetBotChannel(ctx context.Context, c BotChannel) error {
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"bot_is_admin", "changed_at"}),
		Where: clause.Where{
			Exprs: []clause.Expression{clause.Expr{SQL: "bot_channels.changed_at <= excluded.changed_at"}},
		},
	}).Create(&c).Error
	if err != nil {
		return fmt.Errorf("save bot status in chat %d: %w", c.ChatID, err)
	}

	return nil
}

// AdminChannels returns the IDs of the channels where the bot is an admin now.
func (s *Store) AdminChannels(ctx context.Context) ([]int64, error) {
	var ids []int64

	err := s.db.WithContext(ctx).
		Model(&BotChannel{}).
		Where("bot_is_admin").
		Order("chat_id").
		Pluck("chat_id", &ids).
		Error
	if err != nil {
		return nil, fmt.Errorf("list channels where the bot is an admin: %w", err)
	}

	return ids, nil
}

// KnownChannels returns the IDs of every channel the bot has a row about: in bot_channels,
// the audience log or the giveaways. Each table has an index that starts with chat_id. It
// names the giveaways table directly, because importing package giveaway here would make an
// import cycle as soon as giveaway needs this package.
func (s *Store) KnownChannels(ctx context.Context) ([]int64, error) {
	var ids []int64

	err := s.db.WithContext(ctx).Raw(`SELECT chat_id FROM bot_channels
		UNION SELECT chat_id FROM channel_member_events
		UNION SELECT chat_id FROM channel_posts
		UNION SELECT chat_id FROM giveaways
		ORDER BY chat_id`).Scan(&ids).Error
	if err != nil {
		return nil, fmt.Errorf("list known channels: %w", err)
	}

	return ids, nil
}
