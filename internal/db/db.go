// Package db opens the bot's database and brings its tables up to date.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/alexander-danilenko/read_wine_bot/internal/config"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// sqlitePragmas turn on foreign keys, which SQLite leaves off, and let readers and the writer
// work at the same time instead of failing with "database is locked".
const sqlitePragmas = "_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

// ErrUnknownDriver means driver is neither config.DriverSQLite nor config.DriverPostgres.
var ErrUnknownDriver = errors.New("unknown database driver")

// Open connects to the database, pings it and creates or updates the bot's tables. For
// config.DriverSQLite, dsn is a file path; for config.DriverPostgres, a connection string.
// The caller closes the connection with DB().Close().
func Open(ctx context.Context, driver, dsn string) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch driver {
	case config.DriverSQLite:
		dialector = sqlite.Open(dsn + "?" + sqlitePragmas)
	case config.DriverPostgres:
		dialector = postgres.Open(dsn)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownDriver, driver)
	}

	// Errors come back as return values, so GORM's own logger stays silent.
	conn, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", driver, err)
	}

	sqlDB, err := conn.DB()
	if err != nil {
		return nil, fmt.Errorf("get %s connection pool: %w", driver, err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()

		return nil, fmt.Errorf("ping %s database: %w", driver, err)
	}

	if err := conn.WithContext(ctx).AutoMigrate(&user.User{}); err != nil {
		_ = sqlDB.Close()

		return nil, fmt.Errorf("migrate %s database: %w", driver, err)
	}

	return conn, nil
}
