package giveaway

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// Codes of the failures the Mini App can show. Each names what went wrong; Field names the
// form field it concerns, when there is one.
const (
	CodeNotFound          = "not_found"           // no such giveaway, or not the caller's
	CodeNotYours          = "not_yours"           // the giveaway belongs to another admin
	CodeFinished          = "finished"            // the giveaway's winners are drawn; it cannot change
	CodeNotAdmin          = "not_admin"           // the caller is not an admin of the channel
	CodeBotNoRights       = "bot_no_rights"       // the bot cannot edit the post or post in the channel
	CodePostGone          = "post_gone"           // the channel post was deleted
	CodeExists            = "exists"              // the caller's own giveaway is already on the post
	CodeTaken             = "taken"               // another giveaway is already on the post
	CodeInvalidPostLink   = "invalid_post_link"   // not a link to a channel post the bot can read
	CodeInvalidEnd        = "invalid_end"         // the end is not a date and time
	CodeEndInPast         = "end_in_past"         // the end has passed
	CodeInvalidWinners    = "invalid_winners"     // the winner count is out of range
	CodeInvalidButtonText = "invalid_button_text" // the button text is blank or too long
	CodeOtherChannel      = "other_channel"       // a giveaway can move only within its channel
	CodeDue               = "due"                 // past its end, waiting for the draw: it cannot move
	CodeStale             = "stale"               // someone changed the giveaway first; reload it
	CodeFailed            = "failed"              // a database or Telegram failure; try again
)

// Form fields a AppError can concern.
const (
	FieldPostLink = "post_link"
	FieldEndsAt   = "ends_at"
)

// AppError is why a Mini App action failed.
type AppError struct {
	Code  string
	Field string // the form field, or "" when the problem concerns no single field
}

func (p *AppError) Error() string {
	if p.Field == "" {
		return p.Code
	}

	return p.Code + ": " + p.Field
}

// noticeProblems maps the failure notices of the Editor rules to Mini App problems.
func noticeProblems() map[i18n.Key]AppError {
	return map[i18n.Key]AppError{
		i18n.EditNotFound:              {Code: CodeNotFound},
		i18n.EditNotYours:              {Code: CodeNotYours},
		i18n.EditFinished:              {Code: CodeFinished},
		i18n.EditNotAdmin:              {Code: CodeNotAdmin},
		i18n.EditNoRights:              {Code: CodeBotNoRights},
		i18n.EditPostGone:              {Code: CodePostGone},
		i18n.EditMoveTaken:             {Code: CodeTaken, Field: FieldPostLink},
		i18n.EditMoveOtherChannel:      {Code: CodeOtherChannel, Field: FieldPostLink},
		i18n.EditEndPassed:             {Code: CodeEndInPast, Field: FieldEndsAt},
		i18n.GiveawayInvalidButtonText: {Code: CodeInvalidButtonText, Field: FieldButtonText},
	}
}

// successNotices are the notices the Editor rules return when an action worked.
func successNotices() []i18n.Key {
	return []i18n.Key{i18n.GiveawayCreated, i18n.EditCancelled, i18n.EditRestored, i18n.EditFinishing, i18n.EditMoved}
}

// replyErr returns the AppError a rule's Reply stands for, or nil when the action worked.
func replyErr(r Reply) error {
	if r.Stale {
		return &AppError{Code: CodeStale}
	}

	if r.Notice == "" {
		return nil
	}

	if slices.Contains(successNotices(), r.Notice) {
		return nil
	}

	if p, ok := noticeProblems()[r.Notice]; ok {
		return &p
	}

	return &AppError{Code: CodeFailed}
}

// startErr returns the AppError a post check outcome stands for, or nil for Created.
func startErr(r StartResult) error {
	switch r {
	case Created:
		return nil
	case Exists:
		return &AppError{Code: CodeExists, Field: FieldPostLink}
	case Taken:
		return &AppError{Code: CodeTaken, Field: FieldPostLink}
	case NotAdmin:
		return &AppError{Code: CodeNotAdmin, Field: FieldPostLink}
	case NoRights:
		return &AppError{Code: CodeBotNoRights, Field: FieldPostLink}
	case Failed:
		return &AppError{Code: CodeFailed}
	default:
		return &AppError{Code: CodeFailed}
	}
}

// Status is where a giveaway is, as the Mini App shows it.
type Status string

// Statuses.
const (
	StatusActive    Status = "active"    // taking participants
	StatusDue       Status = "due"       // past its end, waiting for the draw within a minute
	StatusCancelled Status = "cancelled" // taken off its post by the creator
	StatusReview    Status = "review"    // drawn; the results wait for review
	StatusFinished  Status = "finished"  // drawn; the results were published or skipped
)

// View is a giveaway as the Mini App shows it. EndsAt is Kyiv wall-clock time, "" for none.
type View struct {
	ID           uint
	ChatID       int64
	MessageID    int64
	PostLink     string
	EndsAt       string
	Winners      int
	ButtonText   string
	Status       Status
	Participants int
}

// AppTimeLayout is how the Mini App writes an end time: Kyiv wall-clock time, as the
// browser's datetime-local input does.
const AppTimeLayout = "2006-01-02T15:04"

// Fields are the values of the Mini App's giveaway form.
type Fields struct {
	PostLink   string
	EndsAt     string // AppTimeLayout in Kyiv
	Winners    int
	ButtonText string
}

// Changes are the fields an edit sets; nil leaves a field as it is.
type Changes struct {
	EndsAt     *string
	Winners    *int
	ButtonText *string
}

// AppTelegram is what the Mini App's actions need from Telegram. BotAPI implements it.
type AppTelegram interface {
	Telegram
	ButtonRemover
	// ResolveChannel returns the chat ID of a channel named as parsePostLink returns it, or
	// ErrNotChannel.
	ResolveChannel(ctx context.Context, chat string) (int64, error)
}

// RunningLister reads a creator's running giveaways page by page. *Store implements it.
type RunningLister interface {
	Running(ctx context.Context, userID int64, offset, limit int) ([]Giveaway, int, error)
}

// App runs the giveaway rules for the Mini App: only the creator changes a giveaway, and the
// join button changes before the row (ADR 0006).
type App struct {
	ed       *Editor
	reviewer *Reviewer
	running  RunningLister
	log      *slog.Logger
}

// NewApp returns the Mini App's entry to the rules of ed and reviewer.
func NewApp(ed *Editor, reviewer *Reviewer, running RunningLister) *App {
	return &App{ed: ed, reviewer: reviewer, running: running, log: ed.svc.log}
}

// Page is one page of a giveaway list and how many giveaways the whole list holds.
type Page struct {
	Views []View
	Total int
}

// List returns up to limit of the creator's running giveaways after the first offset, newest
// first, and the giveaways whose results wait for review in a channel the user is an admin of,
// the creator's own included.
func (a *App) List(ctx context.Context, tg Telegram, userID int64, offset, limit int) (Page, []View, error) {
	mine, total, err := a.running.Running(ctx, userID, offset, limit)
	if err != nil {
		return Page{}, nil, err //nolint:wrapcheck // Store already names the user
	}

	review, err := a.ed.pendingResults(ctx, tg, userID)
	if err != nil {
		return Page{}, nil, err
	}

	// One participant count for both lists.
	views, err := a.views(ctx, slices.Concat(mine, review))
	if err != nil {
		return Page{}, nil, err
	}

	return Page{Views: views[:len(mine)], Total: total}, views[len(mine):], nil
}

// Archived returns up to limit of the creator's finished and cancelled giveaways after the
// first offset, the latest to leave the running list first.
func (a *App) Archived(ctx context.Context, userID int64, offset, limit int) (Page, error) {
	gs, total, err := a.ed.archived.Archived(ctx, userID, offset, limit)
	if err != nil {
		return Page{}, err //nolint:wrapcheck // Store already names the user
	}

	views, err := a.views(ctx, gs)
	if err != nil {
		return Page{}, err
	}

	return Page{Views: views, Total: total}, nil
}

// Channel is a channel as the giveaway lists show it, read live from Telegram, never stored.
type Channel struct {
	ID      int64
	Title   string
	PhotoID string // Telegram file ID of the small channel photo, "" for none
}

// Channels returns the channels of the giveaways in lists, each once. A title or photo that
// cannot be read is left empty: the lists still work without it.
func (a *App) Channels(ctx context.Context, tg ChannelReader, lists ...[]View) []Channel {
	var channels []Channel

	seen := map[int64]bool{}

	for _, v := range slices.Concat(lists...) {
		if seen[v.ChatID] {
			continue
		}

		seen[v.ChatID] = true
		c := Channel{ID: v.ChatID}

		info, err := tg.Channel(ctx, v.ChatID)
		if err != nil {
			a.log.WarnContext(ctx, "read channel", "chat_id", v.ChatID, "error", err)
		}

		c.Title, c.PhotoID = info.Title, info.PhotoID
		channels = append(channels, c)
	}

	return channels
}

// Get returns giveaway id in any state, but only to its creator: anyone else gets not_found,
// as for a missing ID, so the answer does not reveal which IDs exist.
func (a *App) Get(ctx context.Context, userID int64, id uint) (View, error) {
	g, err := a.ed.giveaways.ByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return View{}, &AppError{Code: CodeNotFound}
	}

	if err != nil {
		return View{}, err //nolint:wrapcheck // Store already names the giveaway
	}

	isDraft := !g.Published && g.CancelledAt == nil
	if g.CreatedBy != userID || isDraft {
		return View{}, &AppError{Code: CodeNotFound}
	}

	return a.view(ctx, g)
}

// Create puts a new giveaway on the post f links to, after the post, admin and rights checks. The
// draft row is only a step inside this call (ADR 0006: button first, then row): the user's
// earlier draft is deleted, the new one saved, the button attached and the row published. A
// failure deletes the draft again, so an abandoned form leaves no row.
func (a *App) Create(ctx context.Context, tg AppTelegram, userID int64, lang string, f Fields) (View, error) {
	svc := a.ed.svc
	log := a.log.With("user_id", userID)

	d, err := a.parseFields(ctx, tg, log, f)
	if err != nil {
		return View{}, err
	}

	d.CreatedBy, d.Language = userID, lang

	if result, ok := svc.checkPost(ctx, tg, userID, d.ChatID, d.MessageID); !ok {
		return View{}, startErr(result)
	}

	if err = svc.drafts.DeleteDraft(ctx, userID); err != nil {
		log.ErrorContext(ctx, "delete earlier giveaway draft", "error", err)

		return View{}, &AppError{Code: CodeFailed}
	}

	if err = svc.drafts.SaveDraft(ctx, &d); err != nil {
		if errors.Is(err, ErrExists) {
			return View{}, &AppError{Code: CodeTaken, Field: FieldPostLink}
		}

		log.ErrorContext(ctx, "save giveaway draft", "error", err)

		return View{}, &AppError{Code: CodeFailed}
	}

	// SaveDraft inserts from a map, so the new ID comes from reading the draft back.
	saved, err := svc.drafts.Draft(ctx, userID)
	if err != nil {
		log.ErrorContext(ctx, "read giveaway draft", "error", err)

		return View{}, &AppError{Code: CodeFailed}
	}

	if err := a.publish(ctx, tg, log, saved); err != nil {
		if delErr := svc.drafts.DeleteDraft(ctx, userID); delErr != nil {
			log.ErrorContext(ctx, "delete failed giveaway draft", "error", delErr)
		}

		return View{}, err
	}

	return a.Get(ctx, userID, saved.ID)
}

// checkWinners accepts minWinners to maxWinners winners.
func checkWinners(n int) error {
	if n < minWinners || n > maxWinners {
		return &AppError{Code: CodeInvalidWinners, Field: FieldWinners}
	}

	return nil
}

// Edit changes the fields c sets on the creator's unfinished giveaway id.
func (a *App) Edit(ctx context.Context, tg Telegram, userID int64, id uint, c Changes) (View, error) {
	g, r, ok := a.ed.load(ctx, tg, userID, id)
	if !ok {
		return View{}, replyErr(r)
	}

	changed := g

	if c.EndsAt != nil {
		ends, err := a.parseEnd(*c.EndsAt)
		if err != nil {
			return View{}, err
		}

		changed.EndsAt = utcPtr(ends)
	}

	if c.Winners != nil {
		if err := checkWinners(*c.Winners); err != nil {
			return View{}, err
		}

		changed.Winners = *c.Winners
	}

	if c.ButtonText != nil {
		text, key := parseButtonText(*c.ButtonText)
		if key != "" {
			return View{}, replyErr(Reply{Notice: key})
		}

		changed.ButtonText = text
	}

	if err := replyErr(a.ed.save(ctx, tg, a.log.With("user_id", userID, "giveaway_id", id), g, changed)); err != nil {
		return View{}, err
	}

	return a.Get(ctx, userID, id)
}

// Cancel takes the creator's published giveaway id off its post.
func (a *App) Cancel(ctx context.Context, tg AppTelegram, userID int64, id uint) (View, error) {
	return a.after(ctx, userID, id, a.ed.Cancel(ctx, tg, tg, userID, id))
}

// Restore puts the creator's cancelled giveaway id back on its post.
func (a *App) Restore(ctx context.Context, tg Telegram, userID int64, id uint) (View, error) {
	return a.after(ctx, userID, id, a.ed.Restore(ctx, tg, userID, id))
}

// FinishNow moves the end of the creator's published giveaway id to now; the next minute's
// draw finishes it.
func (a *App) FinishNow(ctx context.Context, tg Telegram, userID int64, id uint) (View, error) {
	return a.after(ctx, userID, id, a.ed.FinishNow(ctx, tg, userID, id))
}

// Move puts the creator's giveaway id on the post link points to, in the same channel,
// keeping its participants.
func (a *App) Move(ctx context.Context, tg AppTelegram, userID int64, id uint, link string) (View, error) {
	log := a.log.With("user_id", userID, "giveaway_id", id)

	g, r, ok := a.ed.load(ctx, tg, userID, id)
	if !ok {
		return View{}, replyErr(r)
	}

	if a.ed.due(g) {
		return View{}, &AppError{Code: CodeDue}
	}

	chatID, messageID, err := a.resolvePost(ctx, tg, log, link)
	if err != nil {
		return View{}, err
	}

	if messageID == g.MessageID && chatID == g.ChatID {
		return a.view(ctx, g)
	}

	if key, ok := a.ed.checkTarget(ctx, log, g, chatID, messageID); !ok {
		return View{}, replyErr(Reply{Notice: key})
	}

	key, _ := a.ed.moveTo(ctx, tg, tg, log, g, messageID)

	return a.after(ctx, userID, id, Reply{Notice: key})
}

// publish attaches the button to the draft's post and marks the draft published.
func (a *App) publish(ctx context.Context, tg Telegram, log *slog.Logger, d Giveaway) error {
	if err := tg.AttachButton(ctx, d.ChatID, d.MessageID, d.ButtonText); err != nil {
		log.WarnContext(ctx, "attach join button", "chat_id", d.ChatID, "message_id", d.MessageID, "error", err)

		if errors.Is(err, ErrPostGone) {
			return &AppError{Code: CodePostGone, Field: FieldPostLink}
		}

		return &AppError{Code: CodeBotNoRights, Field: FieldPostLink}
	}

	if err := a.ed.svc.store.Publish(ctx, d.ID); err != nil {
		if errors.Is(err, ErrNotFound) {
			// Another admin took the post over meanwhile; the button is theirs now.
			return &AppError{Code: CodeTaken, Field: FieldPostLink}
		}

		log.ErrorContext(ctx, "publish giveaway", "giveaway_id", d.ID, "error", err)

		return &AppError{Code: CodeFailed}
	}

	return nil
}

// parseFields checks the form and returns the giveaway it describes.
func (a *App) parseFields(ctx context.Context, tg AppTelegram, log *slog.Logger, f Fields) (Giveaway, error) {
	chatID, messageID, err := a.resolvePost(ctx, tg, log, f.PostLink)
	if err != nil {
		return Giveaway{}, err
	}

	ends, err := a.parseEnd(f.EndsAt)
	if err != nil {
		return Giveaway{}, err
	}

	if err := checkWinners(f.Winners); err != nil {
		return Giveaway{}, err
	}

	text, key := parseButtonText(f.ButtonText)
	if key != "" {
		return Giveaway{}, replyErr(Reply{Notice: key})
	}

	return Giveaway{
		ChatID:     chatID,
		MessageID:  messageID,
		EndsAt:     utcPtr(ends),
		Winners:    f.Winners,
		ButtonText: text,
	}, nil
}

// resolvePost returns the channel and post a t.me link points to.
func (a *App) resolvePost(ctx context.Context, tg AppTelegram, log *slog.Logger, link string) (int64, int64, error) {
	chat, messageID, ok := parsePostLink(link)
	if !ok {
		return 0, 0, &AppError{Code: CodeInvalidPostLink, Field: FieldPostLink}
	}

	chatID, err := tg.ResolveChannel(ctx, chat)
	if err != nil {
		log.InfoContext(ctx, "resolve post link", "chat", chat, "error", err)

		return 0, 0, &AppError{Code: CodeInvalidPostLink, Field: FieldPostLink}
	}

	return chatID, messageID, nil
}

// parseEnd parses an end time in AppTimeLayout as Kyiv time; it must be ahead.
func (a *App) parseEnd(value string) (time.Time, error) {
	ends, err := time.ParseInLocation(AppTimeLayout, strings.TrimSpace(value), a.ed.svc.kyiv)
	if err != nil {
		return time.Time{}, &AppError{Code: CodeInvalidEnd, Field: FieldEndsAt}
	}

	if !ends.After(a.ed.svc.now()) {
		return time.Time{}, &AppError{Code: CodeEndInPast, Field: FieldEndsAt}
	}

	return ends.UTC(), nil
}

// after returns the giveaway as it is after an action, or the action's problem.
func (a *App) after(ctx context.Context, userID int64, id uint, r Reply) (View, error) {
	if err := replyErr(r); err != nil {
		return View{}, err
	}

	return a.Get(ctx, userID, id)
}

// view returns g as the Mini App shows it.
func (a *App) view(ctx context.Context, g Giveaway) (View, error) {
	views, err := a.views(ctx, []Giveaway{g})
	if err != nil {
		return View{}, err
	}

	return views[0], nil
}

// views returns gs as the Mini App shows them, with their participant counts.
func (a *App) views(ctx context.Context, gs []Giveaway) ([]View, error) {
	ids := make([]uint, 0, len(gs))
	for _, g := range gs {
		ids = append(ids, g.ID)
	}

	counts, err := a.ed.counter.Participants(ctx, ids)
	if err != nil {
		return nil, err //nolint:wrapcheck // Store already names the giveaways
	}

	now, kyiv := a.ed.svc.now(), a.ed.svc.kyiv
	views := make([]View, 0, len(gs))

	for _, g := range gs {
		v := View{
			ID: g.ID, ChatID: g.ChatID, MessageID: g.MessageID, PostLink: postLink(g.ChatID, g.MessageID),
			Winners: g.Winners, ButtonText: a.ed.label(g), Status: status(g, now), Participants: counts[g.ID],
		}
		if g.EndsAt != nil {
			v.EndsAt = g.EndsAt.In(kyiv).Format(AppTimeLayout)
		}

		views = append(views, v)
	}

	return views, nil
}

// status returns where g is at now.
func status(g Giveaway, now time.Time) Status {
	switch {
	case g.FinishedAt != nil && g.ResultsPending:
		return StatusReview
	case g.FinishedAt != nil:
		return StatusFinished
	case !g.Published:
		return StatusCancelled
	case g.State(now) == Due:
		return StatusDue
	default:
		return StatusActive
	}
}
