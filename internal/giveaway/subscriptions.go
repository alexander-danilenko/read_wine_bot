package giveaway

import (
	"context"
	"html"
	"log/slog"
	"slices"
	"strings"
	"sync"
)

// checksAtOnce caps the Telegram calls one review sends at the same time.
const checksAtOnce = 8

// titleWidth is how many UTF-16 code units of a channel title a winner line shows.
const titleWidth = 20

// minNameWidth is the narrowest a winner's name gets before the winner lines drop their
// channel titles and keep only the mark.
const minNameWidth = 10

// Marks of a winner's subscription to the giveaway's channel, and of a check that failed.
const (
	markSubscribed    = "✅"
	markNotSubscribed = "⚠️"
	markUnknown       = "❔"
)

// Channels lists the channels where the bot is an admin. *channel.Store implements it.
type Channels interface {
	AdminChannels(ctx context.Context) ([]int64, error)
}

// SubscriptionTelegram is what winner subscriptions need from the Telegram API.
type SubscriptionTelegram interface {
	// Channel returns the title and username of channel chatID.
	Channel(ctx context.Context, chatID int64) (ChannelInfo, error)
	// IsSubscribed reports whether the user is subscribed to channel chatID.
	IsSubscribed(ctx context.Context, chatID, userID int64) (bool, error)
}

// ChannelInfo is what the review shows of a channel.
type ChannelInfo struct {
	ID       int64
	Title    string
	Username string // "" for a channel without a public username
}

// Subscription is whether a winner is subscribed to one channel.
type Subscription int

// Subscription states. The zero value is a check that failed.
const (
	SubscriptionUnknown Subscription = iota
	Subscribed
	NotSubscribed
)

// Subscriptions are the winner subscriptions of one review.
type Subscriptions struct {
	// Channels has the giveaway's channel first, unless its title could not be read.
	Channels []ChannelInfo
	// Of holds one Subscription per entry of Channels for each winner's Telegram ID.
	Of map[int64][]Subscription
}

// Subscriptions checks which channels where the bot is an admin each winner is subscribed to.
// It always includes the giveaway's channel. A channel whose title cannot be read is left
// out; a check that fails is SubscriptionUnknown. Both are logged.
func (r *Reviewer) Subscriptions(
	ctx context.Context,
	tg SubscriptionTelegram,
	g Giveaway,
	winners []Winner,
) Subscriptions {
	log := r.log.With("giveaway_id", g.ID)

	ids, err := r.channels.AdminChannels(ctx)
	if err != nil {
		log.ErrorContext(ctx, "list channels for winner subscriptions", "error", err)
	}

	ids = slices.DeleteFunc(ids, func(id int64) bool { return id == g.ChatID })
	ids = slices.Insert(ids, 0, g.ChatID)

	infos := make([]*ChannelInfo, len(ids)) // nil for a channel whose title cannot be read
	subs := make([][]Subscription, len(winners))

	var wg sync.WaitGroup

	sem := make(chan struct{}, checksAtOnce)
	run := func(f func()) {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			f()
		})
	}

	for c, id := range ids {
		run(func() {
			info, err := tg.Channel(ctx, id)
			if err != nil {
				log.WarnContext(ctx, "read channel for winner subscriptions", "chat_id", id, "error", err)

				return
			}

			infos[c] = &info
		})
	}

	for w, winner := range winners {
		subs[w] = make([]Subscription, len(ids))

		for c, id := range ids {
			run(func() { subs[w][c] = subscription(ctx, tg, log, id, winner.TelegramID) })
		}
	}

	wg.Wait()

	return readable(infos, winners, subs)
}

// subscription checks whether user userID is subscribed to channel chatID.
func subscription(ctx context.Context, tg SubscriptionTelegram, log *slog.Logger, chatID, userID int64) Subscription {
	ok, err := tg.IsSubscribed(ctx, chatID, userID)
	switch {
	case err != nil:
		log.WarnContext(ctx, "check winner subscription", "chat_id", chatID, "user_id", userID, "error", err)

		return SubscriptionUnknown
	case ok:
		return Subscribed
	default:
		return NotSubscribed
	}
}

// readable returns the subscriptions to the channels of infos that are not nil.
func readable(infos []*ChannelInfo, winners []Winner, subs [][]Subscription) Subscriptions {
	res := Subscriptions{Of: make(map[int64][]Subscription, len(winners))}
	for _, info := range infos {
		if info != nil {
			res.Channels = append(res.Channels, *info)
		}
	}

	for w, winner := range winners {
		kept := make([]Subscription, 0, len(res.Channels))
		for c, info := range infos {
			if info != nil {
				kept = append(kept, subs[w][c])
			}
		}

		res.Of[winner.TelegramID] = kept
	}

	return res
}

// mark returns the mark of the winner's subscription to the giveaway's channel.
func (s Subscriptions) mark(g Giveaway, telegramID int64) string {
	if len(s.Channels) == 0 || s.Channels[0].ID != g.ChatID {
		return markUnknown
	}

	switch s.Of[telegramID][0] {
	case Subscribed:
		return markSubscribed
	case NotSubscribed:
		return markNotSubscribed
	case SubscriptionUnknown:
	}

	return markUnknown
}

// line returns the channels the winner is subscribed to, as HTML links with titles shortened
// to titleWidth, and the length the text shows. Unknown checks are marked; channels the winner
// is not subscribed to are left out.
func (s Subscriptions) line(telegramID int64) (string, int) {
	links := make([]string, 0, len(s.Of[telegramID]))
	visible := 0

	for c, sub := range s.Of[telegramID] {
		prefix := ""

		switch sub {
		case NotSubscribed:
			continue
		case SubscriptionUnknown:
			prefix = markUnknown
		case Subscribed:
		}

		title := shorten(s.Channels[c].Title, titleWidth)
		links = append(links, prefix+channelLink(s.Channels[c], title))
		visible += utf16Len(prefix) + utf16Len(title)
	}

	if len(links) == 0 {
		return "", 0
	}

	const sep, lead = ", ", " — "

	visible += utf16Len(lead) + len(sep)*(len(links)-1)

	return lead + strings.Join(links, sep), visible
}

// list returns every channel as a full-title HTML link joined with " · ", and the length the
// text shows.
func (s Subscriptions) list() (string, int) {
	const sep = " · "

	links := make([]string, 0, len(s.Channels))
	visible := 0

	for _, c := range s.Channels {
		links = append(links, channelLink(c, c.Title))
		visible += utf16Len(c.Title)
	}

	visible += utf16Len(sep) * max(0, len(links)-1)

	return strings.Join(links, sep), visible
}

// channelLink returns an HTML link to channel c that shows title: its public username, or
// the t.me/c link for a channel without one, which opens only for its subscribers.
func channelLink(c ChannelInfo, title string) string {
	href := "https://t.me/c/" + channelPart(c.ID)
	if c.Username != "" {
		href = "https://t.me/" + c.Username
	}

	return `<a href="` + href + `">` + html.EscapeString(title) + "</a>"
}
