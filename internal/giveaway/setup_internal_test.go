package giveaway

import (
	"strings"
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

func kyivLocation(t *testing.T) *time.Location {
	t.Helper()

	loc, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}

	return loc
}

func TestParseDate(t *testing.T) {
	kyiv := kyivLocation(t)
	now := time.Date(2026, 10, 9, 19, 55, 0, 0, kyiv)

	tests := []struct {
		input   string
		want    string
		wantErr i18n.Key
	}{
		{input: "2026-10-25", want: "2026-10-25"},
		{input: " 2026-10-09 ", want: "2026-10-09"},
		{input: "25.10.2026", wantErr: i18n.GiveawayInvalidDate},
		{input: "2026-02-30", wantErr: i18n.GiveawayInvalidDate},
		{input: "2026-10-08", wantErr: i18n.GiveawayDateInPast},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			date, errKey := parseDate(tt.input, kyiv, now)

			got := ""
			if !date.IsZero() {
				got = date.Format(dateLayout)
			}

			if got != tt.want || errKey != tt.wantErr {
				t.Errorf("parseDate(%q) = %q, %q; want %q, %q", tt.input, got, errKey, tt.want, tt.wantErr)
			}
		})
	}
}

func TestParseTime(t *testing.T) {
	kyiv := kyivLocation(t)
	now := time.Date(2026, 10, 9, 19, 55, 0, 0, kyiv)

	tests := []struct {
		name    string
		date    string
		input   string
		want    time.Time
		wantErr i18n.Key
	}{
		// Kyiv leaves summer time on the morning of 2026-10-25, so 19:00 is UTC+2 that day.
		{name: "winter time", date: "2026-10-25", input: "19:00", want: time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC)},
		{name: "summer time", date: "2026-10-24", input: "19:00", want: time.Date(2026, 10, 24, 16, 0, 0, 0, time.UTC)},
		{name: "later today", date: "2026-10-09", input: "21:30", want: time.Date(2026, 10, 9, 18, 30, 0, 0, time.UTC)},
		{name: "earlier today", date: "2026-10-09", input: "09:00", wantErr: i18n.GiveawayTimeInPast},
		{name: "hour 24", date: "2026-10-25", input: "24:00", wantErr: i18n.GiveawayInvalidTime},
		{name: "am suffix", date: "2026-10-25", input: "9am", wantErr: i18n.GiveawayInvalidTime},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errKey := parseTime(tt.date, tt.input, kyiv, now)
			if !got.Equal(tt.want) || errKey != tt.wantErr {
				t.Errorf("parseTime(%q, %q) = %v, %q; want %v, %q", tt.date, tt.input, got, errKey, tt.want, tt.wantErr)
			}
		})
	}
}

func TestParseWinners(t *testing.T) {
	tests := []struct {
		input string
		want  int // 0 when the input is rejected
	}{
		{input: "1", want: 1},
		{input: "98", want: 98},
		{input: "7\n", want: 7},
		{input: "0"},
		{input: "99"},
		{input: "abc"},
		{input: "2.5"},
	}

	for _, tt := range tests {
		got, errKey := parseWinners(tt.input)
		if got != tt.want || (tt.want == 0) != (errKey == i18n.GiveawayInvalidWinners) {
			t.Errorf("parseWinners(%q) = %d, %q; want %d", tt.input, got, errKey, tt.want)
		}
	}
}

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
