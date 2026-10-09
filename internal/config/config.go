// Package config loads the bot's settings from environment variables and .env files.
package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	// ErrMissingToken means neither the environment nor any .env file sets TELEGRAM_BOT_TOKEN.
	ErrMissingToken = errors.New("TELEGRAM_BOT_TOKEN is not set")
	// ErrInvalidToken means TELEGRAM_BOT_TOKEN does not have the <bot id>:<secret> form @BotFather issues.
	ErrInvalidToken = errors.New("TELEGRAM_BOT_TOKEN is malformed, want <bot id>:<secret> from @BotFather")
	// ErrMissingDBDriver means DB_DRIVER is not set.
	ErrMissingDBDriver = errors.New("DB_DRIVER is not set")
	// ErrInvalidDBDriver means DB_DRIVER names a database the bot does not support.
	ErrInvalidDBDriver = errors.New("DB_DRIVER is invalid, want " + DriverSQLite + " or " + DriverPostgres)
	// ErrMissingDBDSN means DB_DSN is not set.
	ErrMissingDBDSN = errors.New("DB_DSN is not set")
	// ErrInvalidDBDSN means a SQLite DB_DSN carries a query string; the bot adds its own pragmas.
	ErrInvalidDBDSN = errors.New("DB_DSN is invalid, want a file path without ? for " + DriverSQLite)
)

// Database drivers DB_DRIVER accepts.
const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
)

// tokenPattern matches a @BotFather token: a numeric bot ID, a colon, then a URL-safe secret.
var tokenPattern = regexp.MustCompile(`^\d+:[\w-]+$`)

// Config holds the bot's settings.
type Config struct {
	TelegramBotToken string
	DBDriver         string // DriverSQLite or DriverPostgres
	DBDSN            string // a file path for SQLite, a connection string for Postgres
}

// Load reads settings and validates them. Each setting comes from its environment variable when
// that is non-empty, otherwise from the .env files in dir and its parent directories, where the
// file nearest to dir wins.
func Load(ctx context.Context, dir string) (Config, error) {
	dotenv, err := readDotenvChain(ctx, dir)
	if err != nil {
		return Config{}, err
	}

	get := func(key string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}

		return dotenv[key]
	}

	cfg := Config{
		TelegramBotToken: get("TELEGRAM_BOT_TOKEN"),
		DBDriver:         get("DB_DRIVER"),
		DBDSN:            get("DB_DSN"),
	}
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// validate checks every setting and reports all problems at once. Messages name the setting,
// never its value, so secrets stay out of the logs.
func (c Config) validate() error {
	return errors.Join(
		validateToken(c.TelegramBotToken),
		validateDB(c.DBDriver, c.DBDSN),
	)
}

func validateToken(token string) error {
	switch {
	case token == "":
		return ErrMissingToken
	case !tokenPattern.MatchString(token):
		return ErrInvalidToken
	default:
		return nil
	}
}

func validateDB(driver, dsn string) error {
	var errs []error

	switch driver {
	case DriverSQLite, DriverPostgres:
	case "":
		errs = append(errs, ErrMissingDBDriver)
	default:
		errs = append(errs, ErrInvalidDBDriver)
	}

	switch {
	case dsn == "":
		errs = append(errs, ErrMissingDBDSN)
	case driver == DriverSQLite && strings.Contains(dsn, "?"):
		errs = append(errs, ErrInvalidDBDSN)
	}

	return errors.Join(errs...)
}
