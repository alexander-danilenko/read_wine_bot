package giveaway

import (
	"net/url"
	"strconv"
	"strings"
)

// parsePostLink parses a channel post link and returns the chat in the form getChat takes: a
// public link https://t.me/<username>/<post> gives "@<username>", a private link
// https://t.me/c/<channel>/<post> gives the chat ID, which the link writes without its -100.
func parsePostLink(text string) (string, int64, bool) {
	u, err := url.Parse(strings.TrimSpace(text))
	if err != nil || u.Host != "t.me" {
		return "", 0, false
	}

	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")

	var chat string

	switch {
	case len(parts) == 3 && parts[0] == "c" && positive(parts[1]):
		chat = "-100" + parts[1]
	case len(parts) == 2 && isUsername(parts[0]):
		chat = "@" + parts[0]
	default:
		return "", 0, false
	}

	post := parts[len(parts)-1]
	if !positive(post) {
		return "", 0, false
	}

	messageID, err := strconv.ParseInt(post, 10, 64)

	return chat, messageID, err == nil
}

// positive reports whether s is a number above zero written in plain digits, with no sign.
func positive(s string) bool {
	n, err := strconv.ParseUint(s, 10, 63)

	return err == nil && n > 0
}

// isUsername reports whether s can be a Telegram username: 4-32 Latin letters, digits and
// underscores, starting with a letter.
func isUsername(s string) bool {
	if len(s) < 4 || len(s) > 32 || !isLetter(rune(s[0])) {
		return false
	}

	for _, r := range s {
		if !isLetter(r) && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}

	return true
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
