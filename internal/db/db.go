// Package db opens the bot's PostgreSQL database and brings its tables up to date.
package db

import (
	"context"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/alexander-danilenko/read_wine_bot/internal/channel"
	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// models returns the structs AutoMigrate turns into tables, parents before children.
func models() []any {
	return []any{
		&user.User{}, &user.Revision{}, &giveaway.Giveaway{}, &giveaway.Participant{},
		&channel.Event{}, &channel.Post{}, &channel.BotChannel{},
	}
}

// descriptions documents the tables and named indexes for database tools. Column descriptions
// come from the models' comment: tags, which AutoMigrate applies; GORM has no tag for tables
// or indexes. COMMENT ON takes no bind parameters, so each text is a literal here.
func descriptions() []string {
	return []string{
		`COMMENT ON TABLE users IS 'Telegram users the bot has seen: the latest profile plus the bot''s own state per user'`,
		`COMMENT ON TABLE user_revisions IS 'History of user profiles: a new row each time a user''s Telegram profile changes. Rows are only added'`,
		`COMMENT ON INDEX idx_user_revisions_number IS 'One revision number per user'`,
		`COMMENT ON TABLE giveaways IS 'Giveaways on channel posts, including unfinished setups (unpublished, not cancelled) and cancelled ones'`,
		`COMMENT ON INDEX idx_giveaways_post IS 'At most one giveaway per channel post'`,
		`COMMENT ON INDEX idx_giveaways_draft IS 'At most one unfinished setup (unpublished, not cancelled) per admin'`,
		`COMMENT ON TABLE giveaway_participants IS 'Users who tapped a giveaway''s join button, with the winners marked after the draw'`,
		`COMMENT ON INDEX idx_participants_place IS 'Each place in a giveaway''s results belongs to at most one winner'`,
		`COMMENT ON TABLE channel_member_events IS 'Every subscribe, unsubscribe and removal in the channel since logging started. Rows are only added'`,
		`COMMENT ON INDEX idx_channel_member_events_once IS 'One row per event, so an update Telegram delivers twice is saved once'`,
		`COMMENT ON TABLE channel_posts IS 'Time of every channel post since logging started, to compare unsubscribes with posts. Rows are only added'`,
		`COMMENT ON TABLE bot_channels IS 'Channels the bot was added to and whether it is an admin there now, to check winner subscriptions. Rows stay when the bot is removed'`,
	}
}

// Open connects to the PostgreSQL database at url, pings it and creates or updates the bot's
// tables and their descriptions. The caller closes the connection with DB().Close().
func Open(ctx context.Context, url string) (*gorm.DB, error) {
	// Errors come back as return values, so GORM's own logger stays silent. TranslateError
	// turns a unique violation into gorm.ErrDuplicatedKey, which stores compare with errors.Is.
	conn, err := gorm.Open(postgres.Open(url), &gorm.Config{Logger: logger.Discard, TranslateError: true})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := conn.DB()
	if err != nil {
		return nil, fmt.Errorf("get connection pool: %w", err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()

		return nil, fmt.Errorf("ping database: %w", err)
	}

	if err := migrate(ctx, conn); err != nil {
		_ = sqlDB.Close()

		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return conn, nil
}

func migrate(ctx context.Context, conn *gorm.DB) error {
	tx := conn.WithContext(ctx)
	if err := tx.AutoMigrate(models()...); err != nil {
		return fmt.Errorf("create tables: %w", err)
	}

	for _, stmt := range descriptions() {
		if err := tx.Exec(stmt).Error; err != nil {
			return fmt.Errorf("describe schema: %w", err)
		}
	}

	return nil
}
