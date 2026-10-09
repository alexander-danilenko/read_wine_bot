package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/alexander-danilenko/read_wine_giveaway_bot/internal/config"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     string // TELEGRAM_BOT_TOKEN value; empty means unset
		want    string
		wantErr error
	}{
		{name: "valid", env: "123456:AAH-secret_x", want: "123456:AAH-secret_x"},
		{name: "unset", wantErr: config.ErrMissingToken},
		{name: "no colon", env: "secret-only", wantErr: config.ErrInvalidToken},
		{name: "non-numeric bot id", env: "bot:secret", wantErr: config.ErrInvalidToken},
		{name: "empty secret", env: "123:", wantErr: config.ErrInvalidToken},
		{name: "trailing newline", env: "123:secret\n", wantErr: config.ErrInvalidToken},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TELEGRAM_BOT_TOKEN", tc.env)

			cfg, err := config.Load()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}

			if err != nil && tc.env != "" && strings.Contains(err.Error(), tc.env) {
				t.Errorf("error %q leaks the token value", err)
			}

			if cfg.TelegramBotToken != tc.want {
				t.Errorf("token = %q, want %q", cfg.TelegramBotToken, tc.want)
			}
		})
	}
}
