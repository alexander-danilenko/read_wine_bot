package giveaway

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// Limits of a giveaway's inputs.
const (
	minWinners        = 1
	maxWinners        = 98 // kept from the chat review, whose 98 reroll buttons, Publish and Skip filled 100
	maxButtonTextRune = 64
)

// Reply is the outcome of a rule: the notice for it, or that it no longer applies.
type Reply struct {
	Notice i18n.Key // what happened; "" for nothing to say
	Stale  bool     // the action no longer applies, so nothing changed
}

// parseButtonText accepts a non-blank join button text of at most maxButtonTextRune
// characters; Telegram counts characters, not bytes, so Cyrillic gets the full length.
func parseButtonText(value string) (string, i18n.Key) {
	text := strings.TrimSpace(value)
	if text == "" || utf8.RuneCountInString(text) > maxButtonTextRune {
		return "", i18n.GiveawayInvalidButtonText
	}

	return text, ""
}

// utcPtr returns t in UTC, or nil for the zero time an invalid value leaves.
func utcPtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	t = t.UTC()

	return &t
}

// postLink returns the t.me link to a channel post. Bot API channel IDs are the channel's
// own ID with a -100 prefix, which the link leaves out.
func postLink(chatID, messageID int64) string {
	return "https://t.me/c/" + channelPart(chatID) + "/" + strconv.FormatInt(messageID, 10)
}

// channelPart returns a channel's chat ID as post links write it: without its -100.
func channelPart(chatID int64) string {
	return strings.TrimPrefix(strconv.FormatInt(chatID, 10), "-100")
}
