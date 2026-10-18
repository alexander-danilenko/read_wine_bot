// Package config loads the bot's settings from environment variables and .env files.
package config

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
)

var (
	// ErrMissingToken means neither the environment nor any .env file sets TELEGRAM_BOT_TOKEN.
	ErrMissingToken = errors.New("TELEGRAM_BOT_TOKEN is not set")
	// ErrInvalidToken means TELEGRAM_BOT_TOKEN does not have the <bot id>:<secret> form @BotFather issues.
	ErrInvalidToken = errors.New("TELEGRAM_BOT_TOKEN is malformed, want <bot id>:<secret> from @BotFather")
	// ErrMissingDatabaseURL means DATABASE_URL is not set.
	ErrMissingDatabaseURL = errors.New("DATABASE_URL is not set")
	// ErrInvalidDatabaseURL means DATABASE_URL is not a postgres:// URL with a host.
	ErrInvalidDatabaseURL = errors.New("DATABASE_URL is invalid, want postgres://user:password@host:port/database")
)

// tokenPattern matches a @BotFather token: a numeric bot ID, a colon, then a URL-safe secret.
var tokenPattern = regexp.MustCompile(`^\d+:[\w-]+$`)

// Config holds the bot's settings.
type Config struct {
	TelegramBotToken string
	DatabaseURL      string // the PostgreSQL connection URL
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
		DatabaseURL:      get("DATABASE_URL"),
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
		validateDatabaseURL(c.DatabaseURL),
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

func validateDatabaseURL(raw string) error {
	if raw == "" {
		return ErrMissingDatabaseURL
	}

	// The parse error is dropped: it quotes the URL, password included.
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
		return ErrInvalidDatabaseURL
	}

	return nil
}
