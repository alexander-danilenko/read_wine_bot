package giveaway

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// Find returns the published giveaway on the channel post, or ErrNotFound.
func (s *Store) Find(ctx context.Context, chatID, messageID int64) (Giveaway, error) {
	var g Giveaway

	err := s.db.WithContext(ctx).
		Where("chat_id = ? AND message_id = ? AND published = ?", chatID, messageID, true).Take(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Giveaway{}, ErrNotFound
	}

	if err != nil {
		return Giveaway{}, fmt.Errorf("find giveaway on post %d in chat %d: %w", messageID, chatID, err)
	}

	return g, nil
}

// Publish marks the draft with id as published. It returns ErrNotFound when no unpublished
// giveaway has that id, for example after a second Confirm.
func (s *Store) Publish(ctx context.Context, id uint) error {
	res := s.db.WithContext(ctx).Model(&Giveaway{}).
		Where("id = ? AND published = ?", id, false).Update("published", true)
	if res.Error != nil {
		return fmt.Errorf("publish giveaway %d: %w", id, res.Error)
	}

	if res.RowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// SaveDraft saves g, unpublished, as the draft of g.CreatedBy on its post. An unpublished
// giveaway on the same post is taken over, even from another admin; a published one is left
// alone and SaveDraft returns ErrExists.
//
// The row is inserted from a map: for a struct, GORM replaces the false zero value of
// Published with the column default, true, so every draft would be saved as published.
func (s *Store) SaveDraft(ctx context.Context, g *Giveaway) error {
	now := time.Now()
	row := map[string]any{
		"chat_id": g.ChatID, "message_id": g.MessageID, "created_by": g.CreatedBy, "language": g.Language,
		"ends_at": g.EndsAt, "winners": g.Winners, "button_text": g.ButtonText, "published": false,
		"created_at": now, "updated_at": now,
	}

	res := s.db.WithContext(ctx).Model(&Giveaway{}).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "chat_id"}, {Name: "message_id"}},
		DoUpdates: clause.AssignmentColumns(
			[]string{"created_by", "language", "ends_at", "winners", "button_text", "updated_at"},
		),
		Where: clause.Where{Exprs: []clause.Expression{clause.Eq{Column: "giveaways.published", Value: false}}},
	}).Create(row)
	if res.Error != nil {
		return fmt.Errorf("save giveaway draft on post %d in chat %d: %w", g.MessageID, g.ChatID, res.Error)
	}

	if res.RowsAffected == 0 {
		return ErrExists
	}

	return nil
}

// Draft returns the user's unpublished giveaway, or ErrNotFound.
func (s *Store) Draft(ctx context.Context, userID int64) (Giveaway, error) {
	var g Giveaway

	err := s.db.WithContext(ctx).Where("created_by = ? AND published = ?", userID, false).Take(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Giveaway{}, ErrNotFound
	}

	if err != nil {
		return Giveaway{}, fmt.Errorf("read giveaway draft of user %d: %w", userID, err)
	}

	return g, nil
}

// DeleteDraft deletes the user's unpublished giveaways. A user without one is not an error.
func (s *Store) DeleteDraft(ctx context.Context, userID int64) error {
	err := s.db.WithContext(ctx).Where("created_by = ? AND published = ?", userID, false).Delete(&Giveaway{}).Error
	if err != nil {
		return fmt.Errorf("delete giveaway draft of user %d: %w", userID, err)
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
