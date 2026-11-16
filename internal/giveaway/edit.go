package giveaway

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// Fields of a giveaway that the Mini App form sends.
const (
	FieldWinners    = "winners"
	FieldButtonText = "button_text"
)

// ArchivedLister reads a channel's finished and cancelled giveaways page by page. *Store
// implements it.
type ArchivedLister interface {
	Archived(ctx context.Context, chatID int64, offset, limit int) ([]Giveaway, int, error)
}

// Giveaways reads the giveaways a creator manages. *Store implements it.
type Giveaways interface {
	ByID(ctx context.Context, id uint) (Giveaway, error)
	Post(ctx context.Context, chatID, messageID int64) (Giveaway, error)
}

// Writer changes the giveaways a creator manages. *Store implements it.
type Writer interface {
	SaveFields(
		ctx context.Context,
		id uint,
		endsAt *time.Time,
		winners int,
		buttonText string,
		adminLabel *string,
	) error
	Cancel(ctx context.Context, id uint) error
	Publish(ctx context.Context, id uint) error
}

// Mover puts a giveaway on another post of its channel. *Store implements it.
type Mover interface {
	Move(ctx context.Context, id uint, messageID int64) error
}

// PendingLister finds a channel's giveaways whose results wait for review. *Store implements it.
type PendingLister interface {
	PendingResults(ctx context.Context, chatID int64) ([]Giveaway, error)
}

// Counter counts the participants of giveaways. *Store implements it.
type Counter interface {
	Participants(ctx context.Context, ids []uint) (map[uint]int, error)
}

// ButtonRemover takes the join button off a channel post. A post without a button counts as
// success.
type ButtonRemover interface {
	RemoveButton(ctx context.Context, chatID, messageID int64) error
}

// Editor holds the rules for changing a published giveaway: who may edit it, how each field
// is saved, cancelling and publishing it again, and the primary channel's lists. Every action reads
// the giveaway again and repeats the checks.
type Editor struct {
	svc       *Service
	giveaways Giveaways
	writer    Writer
	mover     Mover
	pending   PendingLister
	counter   Counter
	archived  ArchivedLister
	primary   int64 // the chat ID of the primary channel; giveaways elsewhere are not found
}

// NewEditor returns an Editor that reuses svc's texts, clock and checks, for the giveaways of
// the primary channel.
func NewEditor(
	svc *Service,
	giveaways Giveaways,
	writer Writer,
	mover Mover,
	pending PendingLister,
	counter Counter,
	archived ArchivedLister,
	primary int64,
) *Editor {
	return &Editor{
		svc: svc, giveaways: giveaways, writer: writer, mover: mover, pending: pending, counter: counter,
		archived: archived, primary: primary,
	}
}

// FinishNow moves the end time of the published giveaway id to now, so the next winner draw,
// within a minute, finishes it. A giveaway already due is stale, so a second tap on the same
// confirmation changes nothing.
func (e *Editor) FinishNow(ctx context.Context, tg Telegram, userID int64, id uint) Reply {
	g, r, ok := e.load(ctx, tg, userID, id)
	if !ok {
		return r
	}

	now := e.svc.now()
	if !g.Published || g.State(now) == Due {
		return Reply{Stale: true}
	}

	err := e.writer.SaveFields(ctx, id, utcPtr(now), g.Winners, g.ButtonText, g.AdminLabel)
	if errors.Is(err, ErrNotFound) {
		return Reply{Notice: i18n.EditFinished}
	}

	if err != nil {
		e.svc.log.ErrorContext(ctx, "finish giveaway now", "user_id", userID, "giveaway_id", id, "error", err)

		return Reply{Notice: i18n.EditFailed}
	}

	return Reply{Notice: i18n.EditFinishing}
}

// Cancel takes the join button off the post of the published giveaway id and marks it
// cancelled, keeping its participants. The button goes first: if the database then fails,
// a second cancel finds the post without a button and completes.
func (e *Editor) Cancel(ctx context.Context, tg Telegram, rm ButtonRemover, userID int64, id uint) Reply {
	log := e.svc.log.With("user_id", userID, "giveaway_id", id)

	g, r, ok := e.load(ctx, tg, userID, id)
	if !ok {
		return r
	}

	if !g.Published {
		return Reply{Stale: true}
	}

	if err := rm.RemoveButton(ctx, g.ChatID, g.MessageID); err != nil {
		log.WarnContext(ctx, "remove join button", "error", err)

		return Reply{Notice: i18n.EditNoRights}
	}

	err := e.writer.Cancel(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Reply{Stale: true}
	}

	if err != nil {
		log.ErrorContext(ctx, "cancel giveaway", "error", err)

		return Reply{Notice: i18n.EditFailed}
	}

	return Reply{Notice: i18n.EditCancelled}
}

// Restore puts the join button on the post of the cancelled giveaway or draft id and
// publishes it. Its end time must still be ahead.
func (e *Editor) Restore(ctx context.Context, tg Telegram, userID int64, id uint) Reply {
	log := e.svc.log.With("user_id", userID, "giveaway_id", id)

	g, r, ok := e.load(ctx, tg, userID, id)
	if !ok {
		return r
	}

	if g.Published {
		return Reply{Stale: true}
	}

	if g.EndsAt == nil || !g.EndsAt.After(e.svc.now()) {
		return Reply{Notice: i18n.EditEndPassed}
	}

	if err := tg.AttachButton(ctx, g.ChatID, g.MessageID, e.label(g)); err != nil {
		log.WarnContext(ctx, "attach join button", "error", err)

		return Reply{Notice: attachFailure(err)}
	}

	err := e.writer.Publish(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Reply{Stale: true}
	}

	if err != nil {
		log.ErrorContext(ctx, "publish giveaway again", "error", err)

		return Reply{Notice: i18n.EditFailed}
	}

	if g.CancelledAt == nil {
		return Reply{Notice: i18n.GiveawayCreated}
	}

	return Reply{Notice: i18n.EditRestored}
}

// attachFailure returns the notice for a failed attach: the post is gone, or the bot may not
// edit it.
func attachFailure(err error) i18n.Key {
	if errors.Is(err, ErrPostGone) {
		return i18n.EditPostGone
	}

	return i18n.EditNoRights
}

// save writes the changed fields of g, the post first when its button text changed.
func (e *Editor) save(ctx context.Context, tg Telegram, log *slog.Logger, g, changed Giveaway) Reply {
	if g.Published && changed.ButtonText != g.ButtonText {
		if err := tg.AttachButton(ctx, g.ChatID, g.MessageID, changed.ButtonText); err != nil {
			log.WarnContext(ctx, "change join button text", "error", err)

			return Reply{Notice: attachFailure(err)}
		}
	}

	err := e.writer.SaveFields(ctx, g.ID, changed.EndsAt, changed.Winners, changed.ButtonText, changed.AdminLabel)
	if errors.Is(err, ErrNotFound) {
		return Reply{Notice: i18n.EditFinished}
	}

	if err != nil {
		log.ErrorContext(ctx, "save giveaway fields", "error", err)

		return Reply{Notice: i18n.EditFailed}
	}

	return Reply{}
}

// load reads giveaway id and checks that the user may change it. A giveaway outside the
// primary channel is not found. On failure it returns the Reply that names the failed check,
// and false.
func (e *Editor) load(ctx context.Context, tg Telegram, userID int64, id uint) (Giveaway, Reply, bool) {
	g, err := e.giveaways.ByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Giveaway{}, Reply{Notice: i18n.EditNotFound}, false
	}

	if err != nil {
		e.svc.log.ErrorContext(ctx, "read giveaway to edit", "user_id", userID, "giveaway_id", id, "error", err)

		return Giveaway{}, Reply{Notice: i18n.EditFailed}, false
	}

	if g.ChatID != e.primary {
		return Giveaway{}, Reply{Notice: i18n.EditNotFound}, false
	}

	if r, ok := e.check(ctx, tg, userID, g); !ok {
		return Giveaway{}, r, false
	}

	return g, Reply{}, true
}

// check checks, in order, that the user is a full admin of the primary channel, that they
// created g, that g is not finished and that the bot may edit its posts. It returns the Reply
// that names the failed check and false, or true when every check passes.
func (e *Editor) check(ctx context.Context, tg Telegram, userID int64, g Giveaway) (Reply, bool) {
	if result, ok := e.svc.checkRole(ctx, tg, userID, e.primary); !ok {
		return roleReply(result), false
	}

	switch {
	case g.CreatedBy != userID:
		return Reply{Notice: i18n.EditNotYours}, false
	case g.FinishedAt != nil:
		return Reply{Notice: i18n.EditFinished}, false
	}

	if _, ok := e.svc.checkBot(ctx, tg, userID, g.ChatID); !ok {
		return Reply{Notice: i18n.EditNoRights}, false
	}

	return Reply{}, true
}

// roleReply returns the Reply for a failed role check.
func roleReply(result StartResult) Reply {
	switch result {
	case ReadOnly:
		return Reply{ReadOnly: true}
	case NotAdmin:
		return Reply{Notice: i18n.EditNotAdmin}
	case Created, Exists, Taken, NoRights, Failed:
		return Reply{Notice: i18n.EditFailed}
	default:
		return Reply{Notice: i18n.EditFailed}
	}
}

// checkTarget checks that the post is in g's channel and carries no other giveaway. It
// returns the notice that names the failed check and false, or true when both hold.
func (e *Editor) checkTarget(
	ctx context.Context,
	log *slog.Logger,
	g Giveaway,
	chatID, messageID int64,
) (i18n.Key, bool) {
	if chatID != g.ChatID {
		return i18n.EditMoveOtherChannel, false
	}

	other, err := e.giveaways.Post(ctx, chatID, messageID)
	switch {
	case errors.Is(err, ErrNotFound):
		return "", true
	case err != nil:
		log.ErrorContext(ctx, "find giveaway on new post", "error", err)

		return i18n.EditFailed, false
	case other.ID == g.ID:
		return "", true
	default:
		return i18n.EditMoveTaken, false
	}
}

// moveTo puts g on the post messageID and returns the notice for the result, and whether
// another post could succeed. A published giveaway's button goes on the new post first, so a
// post the bot cannot edit leaves the giveaway where it was, and comes off the old post last;
// a button left behind there joins nothing.
func (e *Editor) moveTo(
	ctx context.Context, tg Telegram, rm ButtonRemover, log *slog.Logger, g Giveaway, messageID int64,
) (i18n.Key, bool) {
	if g.Published {
		if err := tg.AttachButton(ctx, g.ChatID, messageID, e.label(g)); err != nil {
			log.WarnContext(ctx, "attach join button to new post", "error", err)

			return attachFailure(err), true
		}
	}

	err := e.mover.Move(ctx, g.ID, messageID)
	if err != nil {
		// The giveaway stays where it was, so the button just attached must not stay on the new
		// post.
		e.removeButton(ctx, rm, log, g, messageID)
	}

	switch {
	case errors.Is(err, ErrExists):
		return i18n.EditMoveTaken, true
	case errors.Is(err, ErrNotFound):
		return i18n.EditFinished, false
	case err != nil:
		log.ErrorContext(ctx, "move giveaway", "giveaway_id", g.ID, "error", err)

		return i18n.EditFailed, false
	}

	e.removeButton(ctx, rm, log, g, g.MessageID)

	return i18n.EditMoved, false
}

// removeButton takes the join button of the published g off post messageID of its channel.
// A failure only leaves a button that joins nothing, so it is logged, not reported.
func (e *Editor) removeButton(ctx context.Context, rm ButtonRemover, log *slog.Logger, g Giveaway, messageID int64) {
	if !g.Published {
		return
	}

	if err := rm.RemoveButton(ctx, g.ChatID, messageID); err != nil {
		log.WarnContext(ctx, "remove join button", "giveaway_id", g.ID, "post", messageID, "error", err)
	}
}

// label returns the join button text of g: its own text, or the default one.
func (e *Editor) label(g Giveaway) string {
	if g.ButtonText == "" {
		return e.svc.texts.Text(g.Language, i18n.GiveawayButton)
	}

	return g.ButtonText
}

// due reports whether the published g is past its end time and waits for its draw.
func (e *Editor) due(g Giveaway) bool {
	return g.Published && g.State(e.svc.now()) == Due
}
