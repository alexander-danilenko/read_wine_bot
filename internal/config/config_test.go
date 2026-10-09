package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexander-danilenko/read_wine_giveaway_bot/internal/config"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		file    string // .env contents; empty means no file
		env     string // TELEGRAM_BOT_TOKEN value; empty means unset
		want    string
		wantErr error
	}{
		{name: "file only", file: "TELEGRAM_BOT_TOKEN=111:from-file\n", want: "111:from-file"},
		{name: "env only, no file", env: "222:from_env", want: "222:from_env"},
		{
			name: "env overrides file",
			file: "TELEGRAM_BOT_TOKEN=111:from-file\n",
			env:  "222:from_env",
			want: "222:from_env",
		},
		{name: "neither", wantErr: config.ErrMissingToken},
		{name: "file without token", file: "OTHER=x\n", wantErr: config.ErrMissingToken},
		{name: "no colon", env: "secret-only", wantErr: config.ErrInvalidToken},
		{name: "non-numeric bot id", env: "bot:secret", wantErr: config.ErrInvalidToken},
		{name: "empty secret", env: "123:", wantErr: config.ErrInvalidToken},
		{name: "trailing newline", env: "123:secret\n", wantErr: config.ErrInvalidToken},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TELEGRAM_BOT_TOKEN", tc.env)

			path := filepath.Join(t.TempDir(), ".env")
			if tc.file != "" {
				if err := os.WriteFile(path, []byte(tc.file), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			cfg, err := config.Load(t.Context(), path)
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
