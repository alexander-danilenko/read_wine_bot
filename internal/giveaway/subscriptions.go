package giveaway

import (
	"context"
	"log/slog"
	"slices"
	"sync"
)

// checksAtOnce caps the Telegram calls one review sends at the same time.
const checksAtOnce = 8

// Channels lists the channels where the bot is an admin. *channel.Store implements it.
type Channels interface {
	AdminChannels(ctx context.Context) ([]int64, error)
}

// SubscriptionTelegram is what winner subscriptions need from the Telegram API.
type SubscriptionTelegram interface {
	ChannelReader
	// IsSubscribed reports whether the user is subscribed to channel chatID.
	IsSubscribed(ctx context.Context, chatID, userID int64) (bool, error)
}

// ChannelReader reads a channel's title and photo file ID. BotAPI implements it.
type ChannelReader interface {
	// Channel returns the title, username and photo file ID of channel chatID.
	Channel(ctx context.Context, chatID int64) (ChannelInfo, error)
}

// ChannelInfo is what the review shows of a channel.
type ChannelInfo struct {
	ID       int64
	Title    string
	Username string // "" for a channel without a public username
	PhotoID  string // file ID of the small (160x160) channel photo, "" for none
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
