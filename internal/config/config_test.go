package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexander-danilenko/read_wine_giveaway_bot/internal/config"
)

// The .env walk continues above t.TempDir() into the real filesystem, so these tests assume no
// .env file sits in the system temp directory or its parents.
func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string // directory relative to the temp root -> its .env contents
		start   string            // directory Load starts from, relative to the temp root
		env     string            // TELEGRAM_BOT_TOKEN value; empty means unset
		want    string
		wantErr error
	}{
		{name: "env var only", env: "123456:AAH-secret_x", want: "123456:AAH-secret_x"},
		{name: "file in start dir", files: map[string]string{"a": "TELEGRAM_BOT_TOKEN=1:a\n"}, start: "a", want: "1:a"},
		{
			name:  "inherits from parent",
			files: map[string]string{".": "TELEGRAM_BOT_TOKEN=1:root\n", "a/b": "OTHER=x\n"},
			start: "a/b",
			want:  "1:root",
		},
		{
			name:  "nearer file overrides parent",
			files: map[string]string{".": "TELEGRAM_BOT_TOKEN=1:root\n", "a": "TELEGRAM_BOT_TOKEN=2:a\n"},
			start: "a/b",
			want:  "2:a",
		},
		{
			name:    "child dir file ignored",
			files:   map[string]string{"a/b": "TELEGRAM_BOT_TOKEN=3:b\n"},
			start:   "a",
			wantErr: config.ErrMissingToken,
		},
		{
			name:  "env var overrides files",
			files: map[string]string{"a": "TELEGRAM_BOT_TOKEN=2:a\n"},
			start: "a",
			env:   "9:env",
			want:  "9:env",
		},
		{
			name:  "comments, export and quotes",
			files: map[string]string{"a": "# bot\n\nexport TELEGRAM_BOT_TOKEN = \"4:quoted\"\r\n"},
			start: "a",
			want:  "4:quoted",
		},
		{
			name:    "line without equals sign",
			files:   map[string]string{".": "TELEGRAM_BOT_TOKEN=1:root\nnot a pair\n"},
			start:   "a",
			wantErr: config.ErrInvalidDotenv,
		},
		{
			name:    "invalid key",
			files:   map[string]string{"a": "TELEGRAM-BOT-TOKEN=1:a\n"},
			start:   "a",
			wantErr: config.ErrInvalidDotenv,
		},
		{name: "unset", wantErr: config.ErrMissingToken},
		{name: "no colon", env: "secret-only", wantErr: config.ErrInvalidToken},
		{name: "non-numeric bot id", env: "bot:secret", wantErr: config.ErrInvalidToken},
		{name: "empty secret", env: "123:", wantErr: config.ErrInvalidToken},
		{name: "trailing newline", env: "123:secret\n", wantErr: config.ErrInvalidToken},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TELEGRAM_BOT_TOKEN", tc.env)

			root := t.TempDir()
			for dir, contents := range tc.files {
				writeFile(t, filepath.Join(root, dir, ".env"), contents)
			}

			cfg, err := config.Load(t.Context(), filepath.Join(root, tc.start))
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

func writeFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
