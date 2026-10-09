// Package db opens the bot's SQLite database and brings its tables up to date.
package db

import (
	"context"
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// pragmas turn on foreign keys, which SQLite leaves off, and let readers and the writer
// work at the same time instead of failing with "database is locked".
const pragmas = "_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

// Open opens the SQLite database file at path, pings it and creates or updates the bot's
// tables. The caller closes the connection with DB().Close().
func Open(ctx context.Context, path string) (*gorm.DB, error) {
	// Errors come back as return values, so GORM's own logger stays silent.
	conn, err := gorm.Open(sqlite.Open(path+"?"+pragmas), &gorm.Config{Logger: logger.Discard})
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

	if err := conn.WithContext(ctx).
		AutoMigrate(&user.User{}, &user.Revision{}, &giveaway.Giveaway{}, &giveaway.Participant{}); err != nil {
		_ = sqlDB.Close()

		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return conn, nil
}
