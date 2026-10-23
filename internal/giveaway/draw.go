package giveaway

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"

	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// DrawStore finds due giveaways and records their winners. *Store implements it.
type DrawStore interface {
	Due(ctx context.Context, now time.Time) ([]Giveaway, error)
	Candidates(ctx context.Context, giveawayID uint) ([]user.User, error)
	Finish(ctx context.Context, id uint, winnerIDs []int64, now time.Time) error
}

// Filter decides whether a randomly picked participant may win the giveaway. An error stops
// the giveaway's draw with nothing saved; the next pass tries it again.
type Filter func(ctx context.Context, g Giveaway, candidate user.User) (bool, error)

// AcceptAll is the Filter that lets every participant win.
func AcceptAll(context.Context, Giveaway, user.User) (bool, error) { return true, nil }

// Announcer is what the draw needs to close a giveaway: take its join button off and hand its
// results to review.
type Announcer interface {
	ButtonRemover
	// SendReview sends the giveaway's results to its creator for review.
	SendReview(ctx context.Context, g Giveaway)
}

// ReviewSender hands a drawn giveaway's results to its creator for review. *ReviewNotifier
// implements it.
type ReviewSender interface {
	SendReview(ctx context.Context, b *bot.Bot, g Giveaway)
}

// Drawer finishes due giveaways: it picks their winners, records them, removes the join
// button and sends the results to the creator for review.
type Drawer struct {
	store   DrawStore
	filter  Filter
	shuffle func(n int, swap func(i, j int)) // rand.Shuffle; tests fix the order
	reviews ReviewSender
	now     func() time.Time
	log     *slog.Logger
}

// NewDrawer returns a Drawer.
func NewDrawer(
	store DrawStore, filter Filter, shuffle func(n int, swap func(i, j int)),
	reviews ReviewSender, now func() time.Time, log *slog.Logger,
) *Drawer {
	return &Drawer{store: store, filter: filter, shuffle: shuffle, reviews: reviews, now: now, log: log}
}

// DrawDue draws every giveaway that is due now and sends each one's results to its creator
// for review through b. main runs it once a minute with internal/cron.
func (d *Drawer) DrawDue(ctx context.Context, b *bot.Bot) {
	d.drawDue(ctx, drawAnnouncer{telegramAPI: telegramAPI{b: b}, b: b, reviews: d.reviews})
}

// drawDue draws every giveaway that is due now. A failure in one giveaway is logged and the
// pass moves on to the next.
func (d *Drawer) drawDue(ctx context.Context, tg Announcer) {
	now := d.now()

	gs, err := d.store.Due(ctx, now)
	if err != nil {
		d.log.ErrorContext(ctx, "list due giveaways", "error", err)

		return
	}

	for _, g := range gs {
		if ctx.Err() != nil {
			return
		}

		d.draw(ctx, tg, g, now)
	}
}

func (d *Drawer) draw(ctx context.Context, tg Announcer, g Giveaway, now time.Time) {
	log := d.log.With("giveaway_id", g.ID)

	winners, err := d.pick(ctx, g)
	if err != nil {
		log.ErrorContext(ctx, "pick giveaway winners", "error", err)

		return
	}

	ids := make([]int64, 0, len(winners))
	for _, w := range winners {
		ids = append(ids, w.TelegramID)
	}

	err = d.store.Finish(ctx, g.ID, ids, now)
	if errors.Is(err, ErrNotFound) {
		return // cancelled, finished or given a later end time since Due found it
	}

	if err != nil {
		log.ErrorContext(ctx, "save giveaway winners", "error", err)

		return
	}

	log.InfoContext(ctx, "giveaway drawn", "winners", len(ids))

	// The winners are saved: announce them even if shutdown has begun.
	d.announce(context.WithoutCancel(ctx), tg, g)
}

// pick returns up to g.Winners distinct participants in random order that pass the filter.
func (d *Drawer) pick(ctx context.Context, g Giveaway) ([]user.User, error) {
	candidates, err := d.store.Candidates(ctx, g.ID)
	if err != nil {
		return nil, err //nolint:wrapcheck // the store's error names the giveaway
	}

	return pickFiltered(ctx, d.filter, d.shuffle, g, candidates, g.Winners)
}

// pickFiltered returns up to n of candidates of g in random order that pass filter. The draw
// and a reroll both pick this way.
func pickFiltered(
	ctx context.Context, filter Filter, shuffle func(n int, swap func(i, j int)),
	g Giveaway, candidates []user.User, n int,
) ([]user.User, error) {
	// Walking a shuffled list draws without replacement, so nobody is picked twice.
	shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })

	picked := make([]user.User, 0, min(n, len(candidates)))
	for _, c := range candidates {
		if len(picked) == n {
			break
		}

		ok, err := filter(ctx, g, c)
		if err != nil {
			return nil, err
		}

		if ok {
			picked = append(picked, c)
		}
	}

	return picked, nil
}

// announce removes the join button and sends the results to the creator for review.
func (d *Drawer) announce(ctx context.Context, tg Announcer, g Giveaway) {
	if err := tg.RemoveButton(ctx, g.ChatID, g.MessageID); err != nil {
		d.log.WarnContext(ctx, "remove join button",
			"giveaway_id", g.ID, "chat_id", g.ChatID, "message_id", g.MessageID, "error", err)
	}

	tg.SendReview(ctx, g)
}

// drawAnnouncer is the Announcer of a real draw: Telegram removes the button, and the review
// sender sends the review through the bot.
type drawAnnouncer struct {
	telegramAPI

	b       *bot.Bot
	reviews ReviewSender
}

func (a drawAnnouncer) SendReview(ctx context.Context, g Giveaway) {
	a.reviews.SendReview(ctx, a.b, g)
}
