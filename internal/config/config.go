// Package config loads the bot's settings from environment variables and .env files.
package config

import (
	"context"
	"errors"
	"fmt"
	"net"
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
	// ErrMissingAppURL means APP_URL is not set.
	ErrMissingAppURL = errors.New("APP_URL is not set")
	// ErrInvalidAppURL means APP_URL is not an https:// URL with a host and without a query or fragment.
	ErrInvalidAppURL = errors.New("APP_URL is invalid, want https://host or https://host/ without a path, ? or #")
	// ErrInvalidListenAddr means LISTEN_ADDR is not a host:port address.
	ErrInvalidListenAddr = errors.New("LISTEN_ADDR is invalid, want host:port or :port")
)

// defaultListenAddr is where the Mini App and its API listen when LISTEN_ADDR is unset. It is
// safe in production: the container publishes this port behind the HTTPS proxy.
const defaultListenAddr = ":8080"

// tokenPattern matches a @BotFather token: a numeric bot ID, a colon, then a URL-safe secret.
var tokenPattern = regexp.MustCompile(`^\d+:[\w-]+$`)

// Config holds the bot's settings.
type Config struct {
	TelegramBotToken string
	DatabaseURL      string // the PostgreSQL connection URL
	AppURL           string // the public https:// URL of the Mini App, as registered with @BotFather
	ListenAddr       string // the host:port the HTTP server listens on
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
		AppURL:           get("APP_URL"),
		ListenAddr:       get("LISTEN_ADDR"),
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultListenAddr
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
		validateAppURL(c.AppURL),
		validateListenAddr(c.ListenAddr),
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

// validateAppURL requires https: Telegram opens Mini Apps only over HTTPS. The bot appends its own
// query to this URL, so it must not carry one, and the app is served only at the root.
func validateAppURL(raw string) error {
	if raw == "" {
		return ErrMissingAppURL
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ErrInvalidAppURL
	}

	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return ErrInvalidAppURL
	}

	return nil
}

func validateListenAddr(addr string) error {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return ErrInvalidListenAddr
	}

	return nil
}
