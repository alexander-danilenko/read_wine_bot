package giveaway

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// editPrefix starts every edit step. It is not under stepPrefix, so the setup's Cancel never
// treats an edit as a setup and deletes the user's draft.
const editPrefix = "edit:"

// EditStepPost is the pending command while the bot waits for the post of the giveaway to
// edit. A field step names the field and the giveaway: edit:<field>:<giveaway ID>.
const EditStepPost = editPrefix + "post"

// Fields of a published or cancelled giveaway that the creator can change. Each is the
// suffix of the setup step that asks for the same value.
const (
	FieldEndDate    = "end_date"
	FieldEndTime    = "end_time"
	FieldWinners    = "winners"
	FieldButtonText = "button_text"
)

// Pseudo steps that name a message to show rather than an input to wait for; they are never
// saved as a pending command.
const (
	menuStep      = "menu"   // the field menu of a giveaway
	askCancelStep = "cancel" // the cancel confirmation of a giveaway
	askFinishStep = "finish" // the finish now confirmation of a giveaway
)

// inEdit reports whether a pending command is an edit step.
func inEdit(pending string) bool { return strings.HasPrefix(pending, editPrefix) }

// editStep returns the step that names what for giveaway id: a field, the menu or the cancel
// confirmation.
func editStep(what string, id uint) string {
	return editPrefix + what + ":" + strconv.FormatUint(uint64(id), 10)
}

// parseEditStep returns the field or pseudo step and the giveaway of an editStep, and false
// for anything else.
func parseEditStep(step string) (string, uint, bool) {
	what, rawID, ok := strings.Cut(strings.TrimPrefix(step, editPrefix), ":")
	if !ok || !inEdit(step) {
		return "", 0, false
	}

	id, err := strconv.ParseUint(rawID, 10, 0)
	if err != nil {
		return "", 0, false
	}

	return what, uint(id), true
}

// isField reports whether what names a field the creator can change.
func isField(what string) bool {
	switch what {
	case FieldEndDate, FieldEndTime, FieldWinners, FieldButtonText:
		return true
	default:
		return false
	}
}

// Giveaways reads the giveaways a creator manages. *Store implements it.
type Giveaways interface {
	ByID(ctx context.Context, id uint) (Giveaway, error)
	Post(ctx context.Context, chatID, messageID int64) (Giveaway, error)
	Running(ctx context.Context, userID int64) ([]Giveaway, error)
}

// Writer changes the giveaways a creator manages. *Store implements it.
type Writer interface {
	SaveFields(ctx context.Context, id uint, endsAt *time.Time, winners int, buttonText string) error
	Cancel(ctx context.Context, id uint) error
	Publish(ctx context.Context, id uint) error
}

// ButtonRemover takes the join button off a channel post. A post without a button counts as
// success.
type ButtonRemover interface {
	RemoveButton(ctx context.Context, chatID, messageID int64) error
}

// Editor holds the rules for changing a published giveaway: who may edit it, how each field
// is saved, cancelling and publishing it again, and the creator's list. Every action reads
// the giveaway again and repeats the checks, because the only state between messages is
// the pending command.
type Editor struct {
	svc       *Service
	giveaways Giveaways
	writer    Writer
}

// NewEditor returns an Editor that reuses svc's steps, texts, clock and parsers.
func NewEditor(svc *Service, giveaways Giveaways, writer Writer) *Editor {
	return &Editor{svc: svc, giveaways: giveaways, writer: writer}
}

// BeginEdit starts an edit for the user: the bot now waits for a forwarded post.
func (e *Editor) BeginEdit(ctx context.Context, userID int64) error {
	if err := e.svc.steps.SetPendingCommand(ctx, userID, EditStepPost); err != nil {
		return err //nolint:wrapcheck // user.Store already names the user and the column
	}

	return nil
}

// ForwardEdit takes the forwarded post and opens the field menu of its giveaway when the
// user may change it.
func (e *Editor) ForwardEdit(ctx context.Context, tg Telegram, userID, chatID, messageID int64) Reply {
	log := e.svc.log.With("user_id", userID, "chat_id", chatID, "message_id", messageID)

	if r, ok := e.svc.claim(ctx, log, userID, EditStepPost, ""); !ok {
		return r
	}

	return e.OpenPost(ctx, tg, userID, chatID, messageID)
}

// OpenPost opens the field menu of the giveaway on the post when the user may change it.
func (e *Editor) OpenPost(ctx context.Context, tg Telegram, userID, chatID, messageID int64) Reply {
	log := e.svc.log.With("user_id", userID, "chat_id", chatID, "message_id", messageID)

	g, err := e.giveaways.Post(ctx, chatID, messageID)
	if errors.Is(err, ErrNotFound) {
		return Reply{Notice: i18n.EditNotFound}
	}

	if err != nil {
		log.ErrorContext(ctx, "find giveaway to edit", "error", err)

		return Reply{Notice: i18n.EditFailed}
	}

	if key, ok := e.check(ctx, tg, userID, g); !ok {
		return Reply{Notice: key}
	}

	return Reply{Next: editStep(menuStep, g.ID)}
}

// Field asks for a new value of field: the bot now waits for it.
func (e *Editor) Field(ctx context.Context, tg Telegram, userID int64, id uint, field string) Reply {
	if !isField(field) {
		return Reply{Stale: true}
	}

	if _, r, ok := e.load(ctx, tg, userID, id); !ok {
		return r
	}

	step := editStep(field, id)
	if err := e.svc.steps.SetPendingCommand(ctx, userID, step); err != nil {
		e.svc.log.ErrorContext(ctx, "start field edit", "user_id", userID, "error", err)

		return Reply{Notice: i18n.EditFailed}
	}

	return Reply{Next: step}
}

// Value saves value, typed or tapped as a default, as the field that step waits for, and
// shows the menu again. An invalid value keeps the step and names the expected format; a
// step the user is no longer at changes nothing. A new button text of a published giveaway
// goes on the post first, so a post the bot cannot edit leaves the giveaway unchanged.
func (e *Editor) Value(ctx context.Context, tg Telegram, userID int64, step, value string) Reply {
	log := e.svc.log.With("user_id", userID, "step", step)

	field, id, ok := parseEditStep(step)
	if !ok || !isField(field) {
		return Reply{Stale: true}
	}

	current, err := e.svc.steps.PendingCommand(ctx, userID)
	if err != nil {
		log.ErrorContext(ctx, "read edit step", "error", err)

		return Reply{Notice: i18n.EditFailed}
	}

	if current != step {
		return Reply{Stale: true}
	}

	g, r, ok := e.load(ctx, tg, userID, id)
	if !ok {
		if err := e.svc.steps.SetPendingCommand(ctx, userID, ""); err != nil {
			log.ErrorContext(ctx, "clear edit step", "error", err)
		}

		return r
	}

	changed := g
	if errKey := e.fill(&changed, field, value); errKey != "" {
		return Reply{Notice: errKey, Next: step}
	}

	if r, ok := e.svc.claim(ctx, log, userID, step, ""); !ok {
		return r
	}

	return e.save(ctx, tg, log, g, changed)
}

// ShowMenu shows the field menu of giveaway id again when the user may still change it.
func (e *Editor) ShowMenu(ctx context.Context, tg Telegram, userID int64, id uint) Reply {
	if _, r, ok := e.load(ctx, tg, userID, id); !ok {
		return r
	}

	return Reply{Next: editStep(menuStep, id)}
}

// AskCancel asks the creator to confirm cancelling the published giveaway id.
func (e *Editor) AskCancel(ctx context.Context, tg Telegram, userID int64, id uint) Reply {
	return e.askPublished(ctx, tg, userID, id, askCancelStep)
}

// AskFinish asks the creator to confirm finishing the published giveaway id now.
func (e *Editor) AskFinish(ctx context.Context, tg Telegram, userID int64, id uint) Reply {
	return e.askPublished(ctx, tg, userID, id, askFinishStep)
}

// FinishNow moves the end time of the published giveaway id to now, so the next winner draw,
// within a minute, finishes it.
func (e *Editor) FinishNow(ctx context.Context, tg Telegram, userID int64, id uint) Reply {
	g, r, ok := e.load(ctx, tg, userID, id)
	if !ok {
		return r
	}

	if !g.Published {
		return Reply{Stale: true}
	}

	err := e.writer.SaveFields(ctx, id, utcPtr(e.svc.now()), g.Winners, g.ButtonText)
	if errors.Is(err, ErrNotFound) {
		return Reply{Notice: i18n.EditFinished}
	}

	if err != nil {
		e.svc.log.ErrorContext(ctx, "finish giveaway now", "user_id", userID, "giveaway_id", id, "error", err)

		return Reply{Notice: i18n.EditFailed, Next: editStep(menuStep, id)}
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

	menu := editStep(menuStep, id)

	if err := rm.RemoveButton(ctx, g.ChatID, g.MessageID); err != nil {
		log.WarnContext(ctx, "remove join button", "error", err)

		return Reply{Notice: i18n.EditNoRights, Next: menu}
	}

	err := e.writer.Cancel(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Reply{Stale: true}
	}

	if err != nil {
		log.ErrorContext(ctx, "cancel giveaway", "error", err)

		return Reply{Notice: i18n.EditFailed, Next: menu}
	}

	return Reply{Notice: i18n.EditCancelled, Next: menu}
}

// Restore puts the join button back on the post of the cancelled giveaway id and publishes
// it again. Its end time must still be ahead.
func (e *Editor) Restore(ctx context.Context, tg Telegram, userID int64, id uint) Reply {
	log := e.svc.log.With("user_id", userID, "giveaway_id", id)

	g, r, ok := e.load(ctx, tg, userID, id)
	if !ok {
		return r
	}

	if g.Published {
		return Reply{Stale: true}
	}

	menu := editStep(menuStep, id)

	if g.EndsAt == nil || !g.EndsAt.After(e.svc.now()) {
		return Reply{Notice: i18n.EditEndPassed, Next: menu}
	}

	label := g.ButtonText
	if label == "" {
		label = e.svc.texts.Text(g.Language, i18n.GiveawayButton)
	}

	if err := tg.AttachButton(ctx, g.ChatID, g.MessageID, label); err != nil {
		log.WarnContext(ctx, "attach join button", "error", err)

		return Reply{Notice: i18n.EditNoRights, Next: menu}
	}

	err := e.writer.Publish(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Reply{Stale: true}
	}

	if err != nil {
		log.ErrorContext(ctx, "publish giveaway again", "error", err)

		return Reply{Notice: i18n.EditFailed, Next: menu}
	}

	return Reply{Notice: i18n.EditRestored, Next: menu}
}

// Stop ends the wait for a forward or a field value: the edit ends at the forward, and the
// menu shows again at a field.
func (e *Editor) Stop(ctx context.Context, userID int64) Reply {
	log := e.svc.log.With("user_id", userID)

	current, err := e.svc.steps.PendingCommand(ctx, userID)
	if err != nil {
		log.ErrorContext(ctx, "read edit step", "error", err)

		return Reply{Notice: i18n.EditFailed}
	}

	if !inEdit(current) {
		return Reply{Stale: true}
	}

	if r, ok := e.svc.claim(ctx, log, userID, current, ""); !ok {
		return r
	}

	if _, id, ok := parseEditStep(current); ok {
		return Reply{Next: editStep(menuStep, id)}
	}

	return Reply{Notice: i18n.EditStopped}
}

// Menu returns the field menu text of giveaway id and whether it is published.
func (e *Editor) Menu(ctx context.Context, id uint, lang string) (string, bool, error) {
	g, err := e.giveaways.ByID(ctx, id)
	if err != nil {
		return "", false, err //nolint:wrapcheck // Store already names the giveaway
	}

	status := i18n.EditStatusCancelled
	if g.Published {
		status = i18n.EditStatusPublished
	}

	text := e.svc.texts.Text(lang, i18n.EditMenu,
		e.svc.texts.Text(lang, status), postLink(g.ChatID, g.MessageID), e.ends(g), g.Winners, g.ButtonText)

	return text, g.Published, nil
}

// List returns the HTML text that lists the user's running giveaways as a numbered list, each
// with a link to its post and a link that opens its field menu in the bot named botUsername.
func (e *Editor) List(ctx context.Context, userID int64, lang, botUsername string) (string, error) {
	gs, err := e.giveaways.Running(ctx, userID)
	if err != nil {
		return "", err //nolint:wrapcheck // Store already names the user
	}

	if len(gs) == 0 {
		return e.svc.texts.Text(lang, i18n.EditListEmpty), nil
	}

	now := e.svc.now()
	lines := make([]string, 0, len(gs))

	for i, g := range gs {
		ends := e.ends(g)
		if g.State(now) == Due {
			ends += e.svc.texts.Text(lang, i18n.EditListDue)
		}

		lines = append(lines, e.svc.texts.Text(lang, i18n.EditListItem,
			i+1, postLink(g.ChatID, g.MessageID), ends, editLink(botUsername, g.ChatID, g.MessageID)))
	}

	return e.svc.texts.Text(lang, i18n.EditList) + "\n\n" + strings.Join(lines, "\n"), nil
}

// Default returns the value a field's default button stands for, or "" if it has none.
func (e *Editor) Default(field, lang string) string {
	return e.svc.Default(stepPrefix+field, lang)
}

// askPublished returns the confirmation step of the published giveaway id.
func (e *Editor) askPublished(ctx context.Context, tg Telegram, userID int64, id uint, step string) Reply {
	g, r, ok := e.load(ctx, tg, userID, id)
	if !ok {
		return r
	}

	if !g.Published {
		return Reply{Stale: true}
	}

	return Reply{Next: editStep(step, id)}
}

// save writes the changed fields of g, the post first when its button text changed.
func (e *Editor) save(ctx context.Context, tg Telegram, log *slog.Logger, g, changed Giveaway) Reply {
	menu := editStep(menuStep, g.ID)

	if g.Published && changed.ButtonText != g.ButtonText {
		if err := tg.AttachButton(ctx, g.ChatID, g.MessageID, changed.ButtonText); err != nil {
			log.WarnContext(ctx, "change join button text", "error", err)

			return Reply{Notice: i18n.EditNoRights, Next: menu}
		}
	}

	err := e.writer.SaveFields(ctx, g.ID, changed.EndsAt, changed.Winners, changed.ButtonText)
	if errors.Is(err, ErrNotFound) {
		return Reply{Notice: i18n.EditFinished}
	}

	if err != nil {
		log.ErrorContext(ctx, "save giveaway fields", "error", err)

		return Reply{Notice: i18n.EditFailed, Next: menu}
	}

	return Reply{Next: menu}
}

// fill parses value as field into g and returns the error text for an invalid value.
func (e *Editor) fill(g *Giveaway, field, value string) i18n.Key {
	var errKey i18n.Key

	switch field {
	case FieldEndDate, FieldEndTime:
		var ends *time.Time
		if ends, errKey = e.newEnd(*g, field, value); errKey == "" {
			g.EndsAt = ends
		}
	case FieldWinners:
		var winners int
		if winners, errKey = parseWinners(value); errKey == "" {
			g.Winners = winners
		}
	case FieldButtonText:
		var text string
		if text, errKey = parseButtonText(value); errKey == "" {
			g.ButtonText = text
		}
	}

	return errKey
}

// newEnd returns g's end time with a new date or time of day. A new date keeps the time of
// day, or takes the default end time when g has none. A new time keeps the date, or takes
// today when g has none or its date has passed, as a due giveaway's has.
func (e *Editor) newEnd(g Giveaway, field, value string) (*time.Time, i18n.Key) {
	kyiv, now := e.svc.kyiv, e.svc.now()
	today := now.In(kyiv).Format(dateLayout)

	date, timeOfDay := e.svc.endDate(g), value
	if field == FieldEndDate {
		parsed, errKey := parseDate(value, kyiv, now)
		if errKey != "" {
			return nil, errKey
		}

		date, timeOfDay = parsed.Format(dateLayout), defaultEndTime
		if g.EndsAt != nil {
			timeOfDay = g.EndsAt.In(kyiv).Format(timeLayout)
		}
	}

	if date < today {
		date = today
	}

	ends, errKey := parseTime(date, timeOfDay, kyiv, now)
	if errKey == i18n.GiveawayTimeInPast && field == FieldEndDate {
		// The date is today, but the kept time of day has passed.
		errKey = i18n.EditDateTimeInPast
	}

	return utcPtr(ends), errKey
}

// ends returns g's end time in Kyiv for display, or "—" for a giveaway without one.
func (e *Editor) ends(g Giveaway) string {
	if g.EndsAt == nil {
		return "—"
	}

	return g.EndsAt.In(e.svc.kyiv).Format(dateLayout + " " + timeLayout)
}

// load reads giveaway id and checks that the user may change it. On failure it returns the
// Reply that names the failed check, and false.
func (e *Editor) load(ctx context.Context, tg Telegram, userID int64, id uint) (Giveaway, Reply, bool) {
	g, err := e.giveaways.ByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Giveaway{}, Reply{Notice: i18n.EditNotFound}, false
	}

	if err != nil {
		e.svc.log.ErrorContext(ctx, "read giveaway to edit", "user_id", userID, "giveaway_id", id, "error", err)

		return Giveaway{}, Reply{Notice: i18n.EditFailed}, false
	}

	if key, ok := e.check(ctx, tg, userID, g); !ok {
		return Giveaway{}, Reply{Notice: key}, false
	}

	return g, Reply{}, true
}

// check checks, in order, that the user created g, that g is not finished, that the user is
// still an admin of its channel and that the bot may edit its posts. It returns the text
// that names the failed check and false, or true when every check passes.
func (e *Editor) check(ctx context.Context, tg Telegram, userID int64, g Giveaway) (i18n.Key, bool) {
	switch {
	case g.CreatedBy != userID:
		return i18n.EditNotYours, false
	case g.FinishedAt != nil:
		return i18n.EditFinished, false
	}

	switch result, _ := e.svc.checkAdmin(ctx, tg, userID, g.ChatID); result {
	case Created:
		return "", true
	case NotAdmin:
		return i18n.EditNotAdmin, false
	case Exists, Taken, NoRights, Failed:
		return i18n.EditNoRights, false
	default:
		return i18n.EditNoRights, false
	}
}
