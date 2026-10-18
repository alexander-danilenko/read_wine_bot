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
