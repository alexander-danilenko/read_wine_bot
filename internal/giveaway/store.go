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

// SaveDraft saves d as the user's draft, replacing every field of an earlier one.
func (s *Store) SaveDraft(ctx context.Context, d *Draft) error {
	res := s.db.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{UpdateAll: true}).Create(d)
	if res.Error != nil {
		return fmt.Errorf("save giveaway draft of user %d: %w", d.UserID, res.Error)
	}

	return nil
}

// Draft returns the user's draft, or ErrNotFound.
func (s *Store) Draft(ctx context.Context, userID int64) (Draft, error) {
	var d Draft

	err := s.db.WithContext(ctx).Take(&d, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Draft{}, ErrNotFound
	}

	if err != nil {
		return Draft{}, fmt.Errorf("read giveaway draft of user %d: %w", userID, err)
	}

	return d, nil
}

// UpdateDraft sets columns of the user's draft; columns maps column names to values.
func (s *Store) UpdateDraft(ctx context.Context, userID int64, columns map[string]any) error {
	err := s.db.WithContext(ctx).Model(&Draft{}).Where("user_id = ?", userID).Updates(columns).Error
	if err != nil {
		return fmt.Errorf("update giveaway draft of user %d: %w", userID, err)
	}

	return nil
}

// DeleteDraft deletes the user's draft. A user without one is not an error.
func (s *Store) DeleteDraft(ctx context.Context, userID int64) error {
	if err := s.db.WithContext(ctx).Delete(&Draft{}, "user_id = ?", userID).Error; err != nil {
		return fmt.Errorf("delete giveaway draft of user %d: %w", userID, err)
	}

	return nil
}
