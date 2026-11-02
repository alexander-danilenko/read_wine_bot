package giveaway

import (
	"strings"
	"testing"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

func TestParseButtonText(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr i18n.Key
	}{
		{name: "64 Cyrillic runes", input: strings.Repeat("ї", 64)},
		{name: "65 runes", input: strings.Repeat("a", 65), wantErr: i18n.GiveawayInvalidButtonText},
		{name: "blank", input: "   ", wantErr: i18n.GiveawayInvalidButtonText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, errKey := parseButtonText(tt.input); errKey != tt.wantErr {
				t.Errorf("parseButtonText error = %q, want %q", errKey, tt.wantErr)
			}
		})
	}
}

func TestParsePostLink(t *testing.T) {
	tests := []struct {
		input     string
		chat      string
		messageID int64
		ok        bool
	}{
		{input: "https://t.me/c/4213611049/20", chat: "-1004213611049", messageID: 20, ok: true},
		{input: " https://t.me/c/4213611049/20?single ", chat: "-1004213611049", messageID: 20, ok: true},
		{input: "https://t.me/read_wine/1973", chat: "@read_wine", messageID: 1973, ok: true},
		{input: "https://t.me/c/4213611049"},
		{input: "https://t.me/c/4213611049/0"},
		{input: "https://t.me/c/-5/20"},
		{input: "https://t.me/c/4213611049/20/3"},
		{input: "https://t.me/read_wine/+5"},
		{input: "https://t.me/rw/20"},
		{input: "https://t.me/+AbCdEf/20"},
		{input: "https://example.com/read_wine/20"},
		{input: "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			chat, messageID, ok := parsePostLink(tt.input)
			if chat != tt.chat || messageID != tt.messageID || ok != tt.ok {
				t.Errorf("parsePostLink(%q) = %q, %d, %v; want %q, %d, %v",
					tt.input, chat, messageID, ok, tt.chat, tt.messageID, tt.ok)
			}
		})
	}
}
