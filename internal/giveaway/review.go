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

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// maxMessageLen is Telegram's limit for one message: UTF-16 code units of the text left after
// the markup is parsed.
const maxMessageLen = 4096

// winnerBullet starts each winner line of the results.
const winnerBullet = "🏆 "

// ReviewStore reads drawn giveaways and their results. *Store implements it.
type ReviewStore interface {
	ByID(ctx context.Context, id uint) (Giveaway, error)
	Post(ctx context.Context, chatID, messageID int64) (Giveaway, error)
	Results(ctx context.Context, id uint) (Results, error)
}

// RerollStore replaces winners during results review. *Store implements it.
type RerollStore interface {
	Eligible(ctx context.Context, id uint) ([]user.User, error)
	Reroll(ctx context.Context, id uint, out, in int64, now time.Time) error
}

// PublishStore claims results for publishing and gives a claim back. *Store implements it.
type PublishStore interface {
	ClaimResults(ctx context.Context, id uint, rerolled int) error
	ReleaseResults(ctx context.Context, id uint) error
}

// ReviewTelegram is what results review needs from the Telegram API.
type ReviewTelegram interface {
	// IsChannelAdmin reports whether the user is the creator or an administrator of the channel.
	IsChannelAdmin(ctx context.Context, chatID, userID int64) (bool, error)
	// SendHTML sends an HTML text to the chat, as a reply to message replyTo unless it is 0.
	// A replyTo that no longer exists sends the text without the reply.
	SendHTML(ctx context.Context, chatID, replyTo int64, text string) error
}

// ReviewReply is the outcome of a results review action: a notice to send, the screen to
// show, or neither for a tap whose button no longer applies.
type ReviewReply struct {
	Notice   i18n.Key // what happened; "" for nothing to say
	Giveaway Giveaway
	Results  Results
	Stale    bool // the action no longer applies, so nothing changed
}

// Reviewer holds the rules of results review: who may review, how a winner is replaced and
// how the results reach the channel.
type Reviewer struct {
	store    ReviewStore
	rerolls  RerollStore
	publish  PublishStore
	channels Channels
	filter   Filter
	shuffle  func(n int, swap func(i, j int))
	texts    *i18n.Catalog
	now      func() time.Time
	log      *slog.Logger
	maxWidth int // Telegram's message length limit; tests lower it
}

// NewReviewer returns a Reviewer. filter and shuffle are the winner draw's, so a replacement
// is picked the way the draw picks winners. channels lists the channels whose subscriptions
// the review shows next to each winner.
func NewReviewer(
	store ReviewStore, rerolls RerollStore, publish PublishStore, channels Channels,
	filter Filter, shuffle func(n int, swap func(i, j int)),
	texts *i18n.Catalog, now func() time.Time, log *slog.Logger,
) *Reviewer {
	return &Reviewer{
		store: store, rerolls: rerolls, publish: publish, channels: channels, filter: filter, shuffle: shuffle,
		texts: texts, now: now, log: log, maxWidth: maxMessageLen,
	}
}

// Show shows the results review of giveaway id.
func (r *Reviewer) Show(ctx context.Context, tg ReviewTelegram, userID int64, id uint) ReviewReply {
	g, res, reply, ok := r.load(ctx, tg, userID, id)
	if !ok {
		return reply
	}

	return ReviewReply{Giveaway: g, Results: res}
}

// Reroll replaces winner out of giveaway id with a random eligible participant who passes
// the filter, or with nobody when none is left, and shows the review again.
func (r *Reviewer) Reroll(ctx context.Context, tg ReviewTelegram, userID int64, id uint, out int64) ReviewReply {
	log := r.log.With("user_id", userID, "giveaway_id", id)

	g, res, reply, ok := r.load(ctx, tg, userID, id)
	if !ok {
		return reply
	}

	if _, ok := winner(res, out); !ok {
		return ReviewReply{Stale: true}
	}

	in, err := r.pick(ctx, g)
	if err != nil {
		log.ErrorContext(ctx, "pick replacement winner", "error", err)

		return ReviewReply{Notice: i18n.EditFailed, Giveaway: g, Results: res}
	}

	err = r.rerolls.Reroll(ctx, id, out, in, r.now())
	if errors.Is(err, ErrNotFound) {
		return ReviewReply{Stale: true}
	}

	if err != nil {
		log.ErrorContext(ctx, "reroll winner", "winner_id", out, "error", err)

		return ReviewReply{Notice: i18n.EditFailed, Giveaway: g, Results: res}
	}

	log.InfoContext(ctx, "winner rerolled", "winner_id", out, "replacement_id", in)

	res, err = r.store.Results(ctx, id)
	if err != nil {
		log.ErrorContext(ctx, "read giveaway results", "error", err)

		return ReviewReply{Notice: i18n.EditFailed}
	}

	return ReviewReply{Giveaway: g, Results: res}
}

// Skip closes giveaway id without posting its results, if they are still the results the
// admin saw: rerolled is their rerolled-out count then. It claims the results the way
// Publish does, so a skip and a publish cannot both win.
func (r *Reviewer) Skip(ctx context.Context, tg ReviewTelegram, userID int64, id uint, rerolled int) ReviewReply {
	log := r.log.With("user_id", userID, "giveaway_id", id)

	g, res, reply, ok := r.load(ctx, tg, userID, id)
	if !ok {
		return reply
	}

	if res.Rerolled != rerolled {
		return ReviewReply{Stale: true}
	}

	err := r.publish.ClaimResults(ctx, id, rerolled)
	if errors.Is(err, ErrNotFound) {
		return ReviewReply{Stale: true}
	}

	if err != nil {
		log.ErrorContext(ctx, "claim giveaway results to skip", "error", err)

		return ReviewReply{Notice: i18n.EditFailed, Giveaway: g, Results: res}
	}

	log.InfoContext(ctx, "giveaway results skipped")

	return ReviewReply{Notice: i18n.ReviewSkipped}
}

// Publish posts the results of giveaway id as a reply to its post, if they are still the
// results the admin saw: rerolled is their rerolled-out count then. The results are claimed
// first, so only one publish posts them, and the claim is given back when the channel
// refuses the first message.
func (r *Reviewer) Publish(ctx context.Context, tg ReviewTelegram, userID int64, id uint, rerolled int) ReviewReply {
	log := r.log.With("user_id", userID, "giveaway_id", id)

	g, res, reply, ok := r.load(ctx, tg, userID, id)
	if !ok {
		return reply
	}

	if res.Rerolled != rerolled {
		return ReviewReply{Stale: true}
	}

	err := r.publish.ClaimResults(ctx, id, rerolled)
	if errors.Is(err, ErrNotFound) {
		return ReviewReply{Stale: true}
	}

	if err != nil {
		log.ErrorContext(ctx, "claim giveaway results", "error", err)

		return ReviewReply{Notice: i18n.EditFailed, Giveaway: g, Results: res}
	}

	// The results are claimed: post them, or give the claim back, even if shutdown has begun.
	ctx = context.WithoutCancel(ctx)
	log = log.With("chat_id", g.ChatID, "message_id", g.MessageID)

	for i, m := range r.resultMessages(g.Language, winnerUsers(res)) {
		err := tg.SendHTML(ctx, g.ChatID, g.MessageID, m)
		if err == nil {
			continue
		}

		log.WarnContext(ctx, "post giveaway results", "part", i+1, "error", err)

		if i > 0 {
			return ReviewReply{Notice: i18n.ReviewPublishedPartly}
		}

		if err := r.publish.ReleaseResults(ctx, id); err != nil {
			log.ErrorContext(ctx, "release giveaway results after a failed post", "error", err)

			return ReviewReply{Notice: i18n.EditFailed}
		}

		return ReviewReply{Notice: i18n.ReviewPublishFailed, Giveaway: g, Results: res}
	}

	log.InfoContext(ctx, "giveaway results published", "winners", len(res.Winners))

	return ReviewReply{Notice: i18n.ReviewPublished}
}

// SendWinners sends the current winners of giveaway id to the user in a private message, for
// them to forward to partners. Each winner links by Telegram ID, so the links work for
// winners without a username. Nothing in the review changes.
func (r *Reviewer) SendWinners(ctx context.Context, tg ReviewTelegram, userID int64, id uint) ReviewReply {
	log := r.log.With("user_id", userID, "giveaway_id", id)

	g, res, reply, ok := r.load(ctx, tg, userID, id)
	if !ok {
		return reply
	}

	// A user's private chat with the bot has the user's ID.
	for i, m := range r.winnersListMessages(g, winnerUsers(res)) {
		if err := tg.SendHTML(ctx, userID, 0, m); err != nil {
			log.WarnContext(ctx, "send winners to the admin", "part", i+1, "error", err)

			return ReviewReply{Notice: i18n.ReviewSendFailed, Giveaway: g, Results: res}
		}
	}

	log.InfoContext(ctx, "winners sent to the admin", "winners", len(res.Winners))

	return ReviewReply{Giveaway: g, Results: res}
}

// load reads giveaway id and its results and checks that they wait for review and that the
// user is an admin of its channel. It returns the reply to send instead and false when a
// check fails.
func (r *Reviewer) load(
	ctx context.Context, tg ReviewTelegram, userID int64, id uint,
) (Giveaway, Results, ReviewReply, bool) {
	log := r.log.With("user_id", userID, "giveaway_id", id)

	g, err := r.store.ByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Giveaway{}, Results{}, ReviewReply{Stale: true}, false
	}

	if err != nil {
		log.ErrorContext(ctx, "read giveaway to review", "error", err)

		return Giveaway{}, Results{}, ReviewReply{Notice: i18n.EditFailed}, false
	}

	if !g.ResultsPending {
		return Giveaway{}, Results{}, ReviewReply{Stale: true}, false
	}

	isAdmin, err := tg.IsChannelAdmin(ctx, g.ChatID, userID)
	if err != nil {
		log.WarnContext(ctx, "check channel admin", "chat_id", g.ChatID, "error", err)

		return Giveaway{}, Results{}, ReviewReply{Notice: i18n.EditFailed}, false
	}

	if !isAdmin {
		return Giveaway{}, Results{}, ReviewReply{Notice: i18n.ReviewNotAdmin}, false
	}

	res, err := r.store.Results(ctx, id)
	if err != nil {
		log.ErrorContext(ctx, "read giveaway results", "error", err)

		return Giveaway{}, Results{}, ReviewReply{Notice: i18n.EditFailed}, false
	}

	return g, res, ReviewReply{}, true
}

// pick returns a random eligible participant of g that passes the filter, or 0 when none is
// left.
func (r *Reviewer) pick(ctx context.Context, g Giveaway) (int64, error) {
	candidates, err := r.rerolls.Eligible(ctx, g.ID)
	if err != nil {
		return 0, err //nolint:wrapcheck // the store's error names the giveaway
	}

	picked, err := pickFiltered(ctx, r.filter, r.shuffle, g, candidates, 1)
	if err != nil || len(picked) == 0 {
		return 0, err
	}

	return picked[0].TelegramID, nil
}

// resultMessages returns the results as HTML messages, each within Telegram's length limit:
// the header, a blank line, one winner per line, a blank line and the congratulations. A
// split starts a new message without the separator.
func (r *Reviewer) resultMessages(lang string, winners []user.User) []string {
	if len(winners) == 0 {
		return []string{html.EscapeString(r.texts.Text(lang, i18n.DrawNoWinners))}
	}

	headerKey, congratsKey := i18n.DrawResults, i18n.DrawCongrats
	if len(winners) == 1 {
		headerKey, congratsKey = i18n.DrawResultsOne, i18n.DrawCongratsOne
	}

	return r.split(plain(r.texts.Text(lang, headerKey)), winnerLines(winners, userLink),
		plain(r.texts.Text(lang, congratsKey)))
}

// winnersListMessages returns the winners list for the admin to forward as HTML messages,
// each within Telegram's length limit: the header linking the post, a blank line and one
// winner per line, each linked by Telegram ID.
func (r *Reviewer) winnersListMessages(g Giveaway, winners []user.User) []string {
	header := r.texts.Text(g.Language, i18n.ReviewWinnersList, postLink(g.ChatID, g.MessageID))

	// The header's markup counts as visible too: overcounting only splits earlier.
	return r.split(part{header, utf16Len(header)}, winnerLines(winners, idLink), part{})
}

// part is a piece of a message: its HTML and how many UTF-16 code units it shows.
type part struct {
	html    string
	visible int
}

// plain returns text as a message part, escaped for HTML.
func plain(text string) part {
	return part{html.EscapeString(text), utf16Len(text)}
}

// winnerLines returns one part per winner: the bullet and the winner linked by link.
func winnerLines(winners []user.User, link func(user.User, int) string) []part {
	lines := make([]part, 0, len(winners))
	for _, w := range winners {
		width := utf16Len(displayName(w))
		lines = append(lines, part{winnerBullet + link(w, width), utf16Len(winnerBullet) + width})
	}

	return lines
}

// split joins header, lines and footer into HTML messages, each within Telegram's length
// limit: the header, a blank line, one line each, then a blank line and the footer unless
// it is empty. A split starts a new message without the separator.
func (r *Reviewer) split(header part, lines []part, footer part) []string {
	var (
		messages []string
		current  string
		size     int
	)

	add := func(sep string, p part) {
		if current != "" && size+len(sep)+p.visible > r.maxWidth {
			messages = append(messages, current)
			current, size = "", 0
		}

		if current != "" {
			current += sep
			size += len(sep)
		}

		current += p.html
		size += p.visible
	}

	add("", header)

	sep := "\n\n"
	for _, l := range lines {
		add(sep, l)
		sep = "\n"
	}

	if footer.html != "" {
		add("\n\n", footer)
	}

	return append(messages, current)
}

// winner returns the current winner with Telegram ID id.
func winner(res Results, id int64) (Winner, bool) {
	for _, w := range res.Winners {
		if w.TelegramID == id {
			return w, true
		}
	}

	return Winner{}, false
}

func winnerUsers(res Results) []user.User {
	us := make([]user.User, 0, len(res.Winners))
	for _, w := range res.Winners {
		us = append(us, w.User)
	}

	return us
}

func displayName(u user.User) string {
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

// userLink returns an HTML link to u that shows u's name, shortened with "…" to at most width
// UTF-16 code units. It links to u's username when u has one: Telegram shows a link by ID as
// plain text when the user's privacy settings hide their account.
func userLink(u user.User, width int) string {
	if u.Username != "" {
		return link("https://t.me/"+u.Username, u, width)
	}

	return idLink(u, width)
}

// idLink is userLink by Telegram ID only, which keeps working after a username changes.
func idLink(u user.User, width int) string {
	return link("tg://user?id="+strconv.FormatInt(u.TelegramID, 10), u, width)
}

func link(href string, u user.User, width int) string {
	return `<a href="` + href + `">` + html.EscapeString(shorten(displayName(u), width)) + "</a>"
}

// shorten cuts s to at most width UTF-16 code units, ending it with "…" when it was cut.
func shorten(s string, width int) string {
	if utf16Len(s) <= width {
		return s
	}

	var b strings.Builder

	n := 1 // the ellipsis
	for _, r := range s {
		n += utf16.RuneLen(r)
		if n > width {
			break
		}

		b.WriteRune(r)
	}

	return b.String() + "…"
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}

	return n
}
