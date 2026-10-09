package giveaway

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNotFound means no giveaway runs on the post.
var ErrNotFound = errors.New("giveaway not found")

// ErrExists means a giveaway already runs on the post.
var ErrExists = errors.New("giveaway already exists")

// Store reads and writes giveaways and their participants.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store backed by db. The tables must already exist.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Find returns the giveaway on the channel post, or ErrNotFound.
func (s *Store) Find(ctx context.Context, chatID, messageID int64) (Giveaway, error) {
	var g Giveaway

	err := s.db.WithContext(ctx).Where("chat_id = ? AND message_id = ?", chatID, messageID).Take(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Giveaway{}, ErrNotFound
	}

	if err != nil {
		return Giveaway{}, fmt.Errorf("find giveaway on post %d in chat %d: %w", messageID, chatID, err)
	}

	return g, nil
}

// Create saves g and fills its ID. It returns ErrExists if a giveaway already runs on the
// same post, including one saved by a concurrent call.
func (s *Store) Create(ctx context.Context, g *Giveaway) error {
	res := s.db.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(g)
	if res.Error != nil {
		return fmt.Errorf("create giveaway on post %d in chat %d: %w", g.MessageID, g.ChatID, res.Error)
	}

	if res.RowsAffected == 0 {
		return ErrExists
	}

	return nil
}

// Join records the user as a participant of the giveaway. It returns false, and no error,
// if the user already participates. The user must already have a row in users.
func (s *Store) Join(ctx context.Context, giveawayID uint, telegramID int64) (bool, error) {
	p := &Participant{GiveawayID: giveawayID, UserID: telegramID}

	res := s.db.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(p)
	if res.Error != nil {
		return false, fmt.Errorf("add user %d to giveaway %d: %w", telegramID, giveawayID, res.Error)
	}

	return res.RowsAffected == 1, nil
}
