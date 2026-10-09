// Package config loads the bot's settings from a .env file and the environment.
package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"

	"github.com/spf13/viper"
)

var (
	// ErrMissingToken means neither the .env file nor the environment sets TELEGRAM_BOT_TOKEN.
	ErrMissingToken = errors.New("TELEGRAM_BOT_TOKEN is not set")
	// ErrInvalidToken means TELEGRAM_BOT_TOKEN does not have the <bot id>:<secret> form @BotFather issues.
	ErrInvalidToken = errors.New("TELEGRAM_BOT_TOKEN is malformed, want <bot id>:<secret> from @BotFather")
)

// tokenPattern matches a @BotFather token: a numeric bot ID, a colon, then a URL-safe secret.
var tokenPattern = regexp.MustCompile(`^\d+:[\w-]+$`)

// Config holds the bot's settings.
type Config struct {
	TelegramBotToken string
}

// Load reads settings from the .env file at path and from the environment, then validates them.
// An environment variable overrides the same key in the file. A missing file is not an error,
// so a container can pass every setting as an environment variable.
func Load(ctx context.Context, path string) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("env")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}

	cfg := Config{TelegramBotToken: v.GetString("TELEGRAM_BOT_TOKEN")}
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
