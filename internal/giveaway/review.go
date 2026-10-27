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

// ReviewScreen is what results review shows next in place of the tapped message.
type ReviewScreen int

// Results review screens.
const (
	NoScreen      ReviewScreen = iota // nothing to show
	ResultsScreen                     // the winners with their reroll buttons and Publish
	RerollScreen                      // asks to confirm replacing one winner
	PublishScreen                     // asks to confirm publishing
	SkipScreen                        // asks to confirm closing without publishing
)

// ReviewReply is the outcome of a results review action: a notice to send, the screen to
// show, or neither for a tap whose button no longer applies.
type ReviewReply struct {
	Notice   i18n.Key     // sent below; "" for none
	Screen   ReviewScreen // shown in place of the tapped message, or below the notice
	Giveaway Giveaway
	Results  Results
	Winner   Winner // the winner RerollScreen asks about
	Stale    bool   // the button no longer applies: the "outdated" popup, nothing else
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

// Open shows the results review of the giveaway on the channel post, from a /list link.
func (r *Reviewer) Open(ctx context.Context, tg ReviewTelegram, userID, chatID, messageID int64) ReviewReply {
	g, err := r.store.Post(ctx, chatID, messageID)
	if errors.Is(err, ErrNotFound) {
		return ReviewReply{Notice: i18n.EditNotFound}
	}

	if err != nil {
		r.log.ErrorContext(ctx, "find giveaway to review",
			"user_id", userID, "chat_id", chatID, "message_id", messageID, "error", err)

		return ReviewReply{Notice: i18n.EditFailed}
	}

	switch {
	case g.FinishedAt == nil:
		return ReviewReply{Notice: i18n.EditNotFound}
	case !g.ResultsPending:
		return ReviewReply{Notice: i18n.ReviewNotPending}
	}

	return r.Show(ctx, tg, userID, g.ID)
}

// Show shows the results review of giveaway id.
func (r *Reviewer) Show(ctx context.Context, tg ReviewTelegram, userID int64, id uint) ReviewReply {
	g, res, reply, ok := r.load(ctx, tg, userID, id)
	if !ok {
		return reply
	}

	return ReviewReply{Screen: ResultsScreen, Giveaway: g, Results: res}
}

// AskReroll asks to confirm replacing winner out of giveaway id.
func (r *Reviewer) AskReroll(ctx context.Context, tg ReviewTelegram, userID int64, id uint, out int64) ReviewReply {
	g, res, reply, ok := r.load(ctx, tg, userID, id)
	if !ok {
		return reply
	}

	w, ok := winner(res, out)
	if !ok {
		return ReviewReply{Stale: true}
	}

	return ReviewReply{Screen: RerollScreen, Giveaway: g, Results: res, Winner: w}
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

		return ReviewReply{Notice: i18n.EditFailed, Screen: ResultsScreen, Giveaway: g, Results: res}
	}

	err = r.rerolls.Reroll(ctx, id, out, in, r.now())
	if errors.Is(err, ErrNotFound) {
		return ReviewReply{Stale: true}
	}

	if err != nil {
		log.ErrorContext(ctx, "reroll winner", "winner_id", out, "error", err)

		return ReviewReply{Notice: i18n.EditFailed, Screen: ResultsScreen, Giveaway: g, Results: res}
	}

	log.InfoContext(ctx, "winner rerolled", "winner_id", out, "replacement_id", in)

	res, err = r.store.Results(ctx, id)
	if err != nil {
		log.ErrorContext(ctx, "read giveaway results", "error", err)

		return ReviewReply{Notice: i18n.EditFailed}
	}

	return ReviewReply{Screen: ResultsScreen, Giveaway: g, Results: res}
}

// AskPublish asks to confirm publishing the results of giveaway id that the admin saw with
// rerolled participants rerolled out.
func (r *Reviewer) AskPublish(ctx context.Context, tg ReviewTelegram, userID int64, id uint, rerolled int) ReviewReply {
	return r.ask(ctx, tg, userID, id, rerolled, PublishScreen)
}

// AskSkip asks to confirm closing giveaway id without publishing the results that the admin
// saw with rerolled participants rerolled out.
func (r *Reviewer) AskSkip(ctx context.Context, tg ReviewTelegram, userID int64, id uint, rerolled int) ReviewReply {
	return r.ask(ctx, tg, userID, id, rerolled, SkipScreen)
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

		return ReviewReply{Notice: i18n.EditFailed, Screen: ResultsScreen, Giveaway: g, Results: res}
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

		return ReviewReply{Notice: i18n.EditFailed, Screen: ResultsScreen, Giveaway: g, Results: res}
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

		return ReviewReply{Notice: i18n.ReviewPublishFailed, Screen: ResultsScreen, Giveaway: g, Results: res}
	}

	log.InfoContext(ctx, "giveaway results published", "winners", len(res.Winners))

	return ReviewReply{Notice: i18n.ReviewPublished}
}

// Results returns the results review of g for its creator, after the draw: no admin check,
// because the bot sends it to the creator itself.
func (r *Reviewer) Results(ctx context.Context, g Giveaway) (ReviewReply, error) {
	res, err := r.store.Results(ctx, g.ID)
	if err != nil {
		return ReviewReply{}, err //nolint:wrapcheck // the store's error names the giveaway
	}

	return ReviewReply{Screen: ResultsScreen, Giveaway: g, Results: res}, nil
}

// ReviewText returns the results review of g as HTML: a header, the channels of subs and
// one numbered line per winner with the winner's subscription mark, name and channels.
// Names are shortened as needed so the text fits one Telegram message; when that would
// leave them too short, the winner lines drop their channels.
func (r *Reviewer) ReviewText(lang string, g Giveaway, res Results, subs Subscriptions) string {
	link := postLink(g.ChatID, g.MessageID)
	if len(res.Winners) == 0 {
		return r.texts.Text(lang, i18n.ReviewNoWinners, link)
	}

	header := r.texts.Text(lang, i18n.ReviewResults, link)
	if len(subs.Channels) > 0 {
		list, _ := subs.list()
		header += "\n" + r.texts.Text(lang, i18n.ReviewChannels, list)
	}

	// The header's markup counts as visible here, which only leaves the names more room
	// than they need.
	room := r.maxWidth - utf16Len(header) - 1
	prefixes := make([]string, len(res.Winners))
	suffixes := make([]string, len(res.Winners))
	suffixLen := 0

	for i, w := range res.Winners {
		prefixes[i] = "\n" + strconv.Itoa(place(w, i)) + ". " + subs.mark(g, w.TelegramID) + " "
		room -= utf16Len(prefixes[i])

		var n int
		suffixes[i], n = subs.line(w.TelegramID)
		suffixLen += n
	}

	if (room-suffixLen)/len(res.Winners) < minNameWidth {
		clear(suffixes)
	} else {
		room -= suffixLen
	}

	width := max(1, room/len(res.Winners))
	lines := make([]string, 0, len(res.Winners))

	for i, w := range res.Winners {
		lines = append(lines, prefixes[i]+userLink(w.User, width)+suffixes[i])
	}

	return header + "\n" + strings.Join(lines, "")
}

// RerollText returns the confirmation for replacing winner w as HTML.
func (r *Reviewer) RerollText(lang string, i int, w Winner) string {
	return r.texts.Text(lang, i18n.ReviewAskReroll, place(w, i), userLink(w.User, r.maxWidth))
}

// ask shows the confirmation screen for the results of giveaway id that the admin saw with
// rerolled participants rerolled out.
func (r *Reviewer) ask(
	ctx context.Context, tg ReviewTelegram, userID int64, id uint, rerolled int, screen ReviewScreen,
) ReviewReply {
	g, res, reply, ok := r.load(ctx, tg, userID, id)
	if !ok {
		return reply
	}

	if res.Rerolled != rerolled {
		return ReviewReply{Stale: true}
	}

	return ReviewReply{Screen: screen, Giveaway: g, Results: res}
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

	var (
		messages []string
		current  string
		size     int
	)

	// add appends text, which shows as plain visible characters, to the current message.
	add := func(sep, text string, visible int) {
		if current != "" && size+len(sep)+visible > r.maxWidth {
			messages = append(messages, current)
			current, size = "", 0
		}

		if current != "" {
			current += sep
			size += len(sep)
		}

		current += text
		size += visible
	}

	headerKey, congratsKey := i18n.DrawResults, i18n.DrawCongrats
	if len(winners) == 1 {
		headerKey, congratsKey = i18n.DrawResultsOne, i18n.DrawCongratsOne
	}

	header := r.texts.Text(lang, headerKey)
	add("", html.EscapeString(header), utf16Len(header))

	sep := "\n\n"
	for _, w := range winners {
		name := displayName(w)
		add(sep, winnerBullet+userLink(w, utf16Len(name)), utf16Len(winnerBullet)+utf16Len(name))
		sep = "\n"
	}

	congrats := r.texts.Text(lang, congratsKey)
	add("\n\n", html.EscapeString(congrats), utf16Len(congrats))

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

// place returns the place the review shows winner w, the i-th in the results, at.
func place(w Winner, i int) int {
	if w.Place > 0 {
		return w.Place
	}

	return i + 1
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
	href := "tg://user?id=" + strconv.FormatInt(u.TelegramID, 10)
	if u.Username != "" {
		href = "https://t.me/" + u.Username
	}

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
