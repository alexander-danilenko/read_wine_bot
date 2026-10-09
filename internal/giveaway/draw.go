package giveaway

import (
	"context"
	"errors"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/go-telegram/bot"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// maxMessageLen is Telegram's limit for one message: UTF-16 code units of the text left after
// the markup is parsed.
const maxMessageLen = 4096

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

// Announcer is what the draw needs from Telegram.
type Announcer interface {
	ButtonRemover
	// SendHTML sends an HTML text to the chat, as a reply to message replyTo unless it is 0.
	SendHTML(ctx context.Context, chatID, replyTo int64, text string) error
}

// Drawer finishes due giveaways: it picks their winners, records them, removes the join
// button and posts the results.
type Drawer struct {
	store   DrawStore
	filter  Filter
	shuffle func(n int, swap func(i, j int)) // rand.Shuffle; tests fix the order
	texts   *i18n.Catalog
	now     func() time.Time
	log     *slog.Logger
}

// NewDrawer returns a Drawer.
func NewDrawer(
	store DrawStore, filter Filter, shuffle func(n int, swap func(i, j int)),
	texts *i18n.Catalog, now func() time.Time, log *slog.Logger,
) *Drawer {
	return &Drawer{store: store, filter: filter, shuffle: shuffle, texts: texts, now: now, log: log}
}

// Run draws the due giveaways at once and then once a minute, until ctx is cancelled.
func (d *Drawer) Run(ctx context.Context, b *bot.Bot) {
	d.run(ctx, telegramAPI{b}, time.Minute)
}

func (d *Drawer) run(ctx context.Context, tg Announcer, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for ctx.Err() == nil {
		d.drawDue(ctx, tg)

		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
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
	d.announce(context.WithoutCancel(ctx), tg, g, winners)
}

// pick returns up to g.Winners distinct participants in random order that pass the filter.
func (d *Drawer) pick(ctx context.Context, g Giveaway) ([]user.User, error) {
	candidates, err := d.store.Candidates(ctx, g.ID)
	if err != nil {
		return nil, err //nolint:wrapcheck // the store's error names the giveaway
	}

	// Walking a shuffled list draws without replacement, so nobody is picked twice.
	d.shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })

	winners := make([]user.User, 0, min(g.Winners, len(candidates)))
	for _, c := range candidates {
		if len(winners) == g.Winners {
			break
		}

		ok, err := d.filter(ctx, g, c)
		if err != nil {
			return nil, err
		}

		if ok {
			winners = append(winners, c)
		}
	}

	return winners, nil
}

// announce removes the join button and replies to the post with the results. If the channel
// refuses a message, the creator gets the post's link and every results message instead.
func (d *Drawer) announce(ctx context.Context, tg Announcer, g Giveaway, winners []user.User) {
	log := d.log.With("giveaway_id", g.ID, "chat_id", g.ChatID, "message_id", g.MessageID)

	if err := tg.RemoveButton(ctx, g.ChatID, g.MessageID); err != nil {
		log.WarnContext(ctx, "remove join button", "error", err)
	}

	messages := d.resultMessages(g.Language, winners)

	for _, m := range messages {
		if err := tg.SendHTML(ctx, g.ChatID, g.MessageID, m); err != nil {
			log.WarnContext(ctx, "post giveaway results", "error", err)
			d.sendToCreator(ctx, tg, g, messages)

			return
		}
	}
}

func (d *Drawer) sendToCreator(ctx context.Context, tg Announcer, g Giveaway, messages []string) {
	link := html.EscapeString(d.texts.Text(g.Language, i18n.DrawPrivatePrefix, postLink(g.ChatID, g.MessageID)))

	for _, m := range append([]string{link}, messages...) {
		if err := tg.SendHTML(ctx, g.CreatedBy, 0, m); err != nil {
			d.log.ErrorContext(ctx, "send giveaway results to creator",
				"giveaway_id", g.ID, "user_id", g.CreatedBy, "error", err)

			return
		}
	}
}

// resultMessages returns the results as HTML messages, each within Telegram's length limit.
// The header goes only in the first.
func (d *Drawer) resultMessages(lang string, winners []user.User) []string {
	if len(winners) == 0 {
		return []string{html.EscapeString(d.texts.Text(lang, i18n.DrawNoWinners))}
	}

	header := d.texts.Text(lang, i18n.DrawResults)
	current := html.EscapeString(header)
	size := utf16Len(header)

	var messages []string

	for i, w := range winners {
		name := strings.TrimSpace(w.FirstName + " " + w.LastName)
		number := strconv.Itoa(i+1) + ". "
		lineSize := utf16Len(number) + utf16Len(name)

		if size+1+lineSize > maxMessageLen { // +1 for the line break before the line
			messages = append(messages, current)
			current, size = "", 0
		}

		if current != "" {
			current += "\n"
			size++
		}

		current += number + `<a href="tg://user?id=` + strconv.FormatInt(w.TelegramID, 10) + `">` +
			html.EscapeString(name) + "</a>"
		size += lineSize
	}

	return append(messages, current)
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}

	return n
}
