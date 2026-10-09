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

// draft selects the unfinished setups: unpublished rows without a cancel mark.
const draft = "published = false AND cancelled_at IS NULL"

// managed selects the giveaways a creator manages: published or cancelled, never a draft.
const managed = "(published = true OR cancelled_at IS NOT NULL)"

// Columns that writes from a map name.
const (
	colPublished  = "published"
	colEndsAt     = "ends_at"
	colCancelled  = "cancelled_at"
	colWinners    = "winners"
	colButtonText = "button_text"
)

// Store reads and writes giveaways and their participants.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store backed by db. The tables must already exist.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Publish marks the draft with id as published. It returns ErrNotFound when no unpublished
// giveaway has that id, for example after a second Confirm.
func (s *Store) Publish(ctx context.Context, id uint) error {
	res := s.db.WithContext(ctx).Model(&Giveaway{}).
		Where("id = ? AND published = ?", id, false).
		Updates(map[string]any{colPublished: true, colCancelled: nil})
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
		colEndsAt: g.EndsAt, colWinners: g.Winners, colButtonText: g.ButtonText, colPublished: false,
		"created_at": now, "updated_at": now,
	}

	res := s.db.WithContext(ctx).Model(&Giveaway{}).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "chat_id"}, {Name: "message_id"}},
		DoUpdates: clause.AssignmentColumns(
			[]string{"created_by", "language", colEndsAt, colWinners, colButtonText, "updated_at"},
		),
		Where: clause.Where{Exprs: []clause.Expression{
			clause.Eq{Column: "giveaways.published", Value: false},
			clause.Eq{Column: "giveaways.cancelled_at", Value: nil},
		}},
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

	err := s.db.WithContext(ctx).Where("created_by = ? AND "+draft, userID).Take(&g).Error
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
	err := s.db.WithContext(ctx).Where("created_by = ? AND "+draft, userID).Delete(&Giveaway{}).Error
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

// Post returns the published or cancelled giveaway on the channel post, or ErrNotFound; a
// setup draft on the post counts as none.
func (s *Store) Post(ctx context.Context, chatID, messageID int64) (Giveaway, error) {
	var g Giveaway

	err := s.db.WithContext(ctx).Where("chat_id = ? AND message_id = ? AND "+managed, chatID, messageID).Take(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Giveaway{}, ErrNotFound
	}

	if err != nil {
		return Giveaway{}, fmt.Errorf("find giveaway on post %d in chat %d: %w", messageID, chatID, err)
	}

	return g, nil
}

// ByID returns the published or cancelled giveaway id, or ErrNotFound; a setup draft counts
// as none.
func (s *Store) ByID(ctx context.Context, id uint) (Giveaway, error) {
	var g Giveaway

	err := s.db.WithContext(ctx).Where("id = ? AND "+managed, id).Take(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Giveaway{}, ErrNotFound
	}

	if err != nil {
		return Giveaway{}, fmt.Errorf("read giveaway %d: %w", id, err)
	}

	return g, nil
}

// Running returns the user's published giveaways that are not finished, soonest end first.
func (s *Store) Running(ctx context.Context, userID int64) ([]Giveaway, error) {
	var gs []Giveaway

	err := s.db.WithContext(ctx).
		Where("created_by = ? AND published = ? AND finished_at IS NULL", userID, true).
		Order(colEndsAt).Order("id").Find(&gs).Error
	if err != nil {
		return nil, fmt.Errorf("list running giveaways of user %d: %w", userID, err)
	}

	return gs, nil
}

// SaveFields saves the end time, winner count and button text of the published or cancelled
// giveaway id. It returns ErrNotFound for a draft, a finished or a missing giveaway.
func (s *Store) SaveFields(ctx context.Context, id uint, endsAt *time.Time, winners int, buttonText string) error {
	res := s.db.WithContext(ctx).Model(&Giveaway{}).
		Where("id = ? AND finished_at IS NULL AND "+managed, id).
		Updates(map[string]any{colEndsAt: endsAt, colWinners: winners, colButtonText: buttonText})
	if res.Error != nil {
		return fmt.Errorf("save fields of giveaway %d: %w", id, res.Error)
	}

	if res.RowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Cancel unpublishes the published, unfinished giveaway id and marks it cancelled, keeping
// its participants. It returns ErrNotFound when no such giveaway exists, so a second cancel
// changes nothing.
func (s *Store) Cancel(ctx context.Context, id uint) error {
	res := s.db.WithContext(ctx).Model(&Giveaway{}).
		Where("id = ? AND published = ? AND finished_at IS NULL", id, true).
		Updates(map[string]any{colPublished: false, colCancelled: time.Now()})
	if res.Error != nil {
		return fmt.Errorf("cancel giveaway %d: %w", id, res.Error)
	}

	if res.RowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}
