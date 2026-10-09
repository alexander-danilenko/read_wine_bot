package giveaway

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// stepPrefix starts every setup step, so a pending command with it means a setup is running.
const stepPrefix = "giveaway:"

// Setup steps, saved as the user's pending command. Each names what the bot waits for next.
const (
	StepPost       = stepPrefix + "post"
	StepEndDate    = stepPrefix + "end_date"
	StepEndTime    = stepPrefix + "end_time"
	StepWinners    = stepPrefix + "winners"
	StepButtonText = stepPrefix + "button_text"
	StepConfirm    = stepPrefix + "confirm"
)

// inSetup reports whether a pending command is a giveaway setup step.
func inSetup(pending string) bool { return strings.HasPrefix(pending, stepPrefix) }

// Limits and defaults of the setup inputs.
const (
	dateLayout        = "2006-01-02"
	timeLayout        = "15:04"
	defaultEndTime    = "19:00"
	minWinners        = 1
	maxWinners        = 100
	maxButtonTextRune = 64
)

// Reply tells the handler what to send after a setup action.
type Reply struct {
	Notice i18n.Key // sent first; "" for none
	Next   string   // the step whose prompt follows; "" when the setup ended
	Stale  bool     // the action belongs to a step that has passed, so nothing changed
}

// Begin starts a new setup for the user: the bot now waits for a forwarded post.
func (s *Service) Begin(ctx context.Context, userID int64) error {
	if err := s.steps.SetPendingCommand(ctx, userID, StepPost); err != nil {
		return err //nolint:wrapcheck // user.Store already names the user and the column
	}

	return nil
}

// Forward takes the forwarded channel post. It claims the step first, so of several posts
// forwarded together only one is checked, then checks the post and saves a new draft.
func (s *Service) Forward(ctx context.Context, tg Telegram, userID, chatID, messageID int64, title string) Reply {
	log := s.log.With("user_id", userID, "chat_id", chatID, "message_id", messageID)

	if r, ok := s.claim(ctx, log, userID, StepPost, StepEndDate); !ok {
		return r
	}

	if result, ok := s.checkPost(ctx, tg, userID, chatID, messageID); !ok {
		s.end(ctx, userID)

		return Reply{Notice: startText(result)}
	}

	err := s.drafts.SaveDraft(ctx, &Draft{UserID: userID, ChatID: chatID, MessageID: messageID, ChannelTitle: title})
	if err != nil {
		log.ErrorContext(ctx, "save giveaway draft", "error", err)
		s.end(ctx, userID)

		return Reply{Notice: i18n.GiveawayFailed}
	}

	return Reply{Next: StepEndDate}
}

// Apply takes the value typed, or the default tapped, at step. An invalid value keeps the
// step and names the expected format; a step the user is not at changes nothing.
func (s *Service) Apply(ctx context.Context, userID int64, step, value string) Reply {
	log := s.log.With("user_id", userID, "step", step)

	current, err := s.steps.PendingCommand(ctx, userID)
	if err != nil {
		log.ErrorContext(ctx, "read setup step", "error", err)

		return Reply{Notice: i18n.GiveawayFailed}
	}

	if current != step {
		return Reply{Stale: true}
	}

	d, err := s.drafts.Draft(ctx, userID)
	if err != nil {
		log.ErrorContext(ctx, "read giveaway draft", "error", err)
		s.end(ctx, userID)

		return Reply{Notice: i18n.GiveawayFailed}
	}

	next, errKey := s.fill(&d, step, value)
	if next == "" {
		return Reply{Stale: true}
	}

	if errKey != "" {
		return Reply{Notice: errKey, Next: step}
	}

	if err := s.drafts.SaveDraft(ctx, &d); err != nil {
		log.ErrorContext(ctx, "save giveaway draft", "error", err)

		return Reply{Notice: i18n.GiveawayFailed, Next: step}
	}

	r, _ := s.claim(ctx, log, userID, step, next)

	return r
}

// fill parses value as the input of step into d and returns the next step, or the error
// text for an invalid value. It returns no next step for a step that takes no value.
func (s *Service) fill(d *Draft, step, value string) (string, i18n.Key) {
	var errKey i18n.Key

	switch step {
	case StepEndDate:
		d.EndDate, errKey = parseDate(value, s.kyiv, s.now())

		return StepEndTime, errKey
	case StepEndTime:
		d.EndsAt, errKey = parseTime(d.EndDate, value, s.kyiv, s.now())

		return StepWinners, errKey
	case StepWinners:
		d.Winners, errKey = parseWinners(value)

		return StepButtonText, errKey
	case StepButtonText:
		d.ButtonText, errKey = parseButtonText(value)

		return StepConfirm, errKey
	default:
		return "", ""
	}
}

// Confirm creates the giveaway from the user's draft. It claims the step first, so a double
// tap attaches one button, then repeats the post check and the end time check, because both
// can change while the summary waits. A passed end time sends the user back to the date or
// time step; missing rights or a failure keep the draft at the confirm step.
func (s *Service) Confirm(ctx context.Context, tg Telegram, userID int64, lang string) Reply {
	log := s.log.With("user_id", userID)

	if r, ok := s.claim(ctx, log, userID, StepConfirm, ""); !ok {
		return r
	}

	d, err := s.drafts.Draft(ctx, userID)
	if err != nil {
		log.ErrorContext(ctx, "read giveaway draft", "error", err)

		return Reply{Notice: i18n.GiveawayFailed}
	}

	if r, ok := s.recheckEnd(ctx, log, userID, d); !ok {
		return r
	}

	result := s.create(ctx, tg, userID, d, lang)

	// Missing rights and failures can be a passing Telegram or database error, so the
	// admin keeps the draft and can confirm again or cancel.
	if result == NoRights || result == Failed {
		if err := s.steps.SetPendingCommand(ctx, userID, StepConfirm); err != nil {
			log.ErrorContext(ctx, "return to setup step", "error", err)

			return Reply{Notice: startText(result)}
		}

		return Reply{Notice: startText(result), Next: StepConfirm}
	}

	if err := s.drafts.DeleteDraft(ctx, userID); err != nil {
		log.ErrorContext(ctx, "delete giveaway draft", "error", err)
	}

	return Reply{Notice: startText(result)}
}

// recheckEnd returns false and the step to go back to when the draft's end time has passed.
func (s *Service) recheckEnd(ctx context.Context, log *slog.Logger, userID int64, d Draft) (Reply, bool) {
	now := s.now()
	if d.EndsAt.After(now) {
		return Reply{}, true
	}

	r := Reply{Notice: i18n.GiveawayTimeInPast, Next: StepEndTime}
	if _, errKey := parseDate(d.EndDate, s.kyiv, now); errKey != "" {
		r = Reply{Notice: i18n.GiveawayDateInPast, Next: StepEndDate}
	}

	if err := s.steps.SetPendingCommand(ctx, userID, r.Next); err != nil {
		log.ErrorContext(ctx, "return to setup step", "error", err)

		return Reply{Notice: i18n.GiveawayFailed}, false
	}

	return r, false
}

// create checks the post again, attaches the button with the draft's text and saves the
// giveaway.
func (s *Service) create(ctx context.Context, tg Telegram, userID int64, d Draft, lang string) StartResult {
	log := s.log.With("user_id", userID, "chat_id", d.ChatID, "message_id", d.MessageID)

	if result, ok := s.checkPost(ctx, tg, userID, d.ChatID, d.MessageID); !ok {
		return result
	}

	if err := tg.AttachButton(ctx, d.ChatID, d.MessageID, d.ButtonText); err != nil {
		log.WarnContext(ctx, "attach join button", "error", err)

		return NoRights
	}

	ends := d.EndsAt.UTC()

	err := s.store.Create(ctx, &Giveaway{
		ChatID: d.ChatID, MessageID: d.MessageID, CreatedBy: userID, Language: lang,
		EndsAt: &ends, Winners: d.Winners, ButtonText: d.ButtonText,
	})
	if errors.Is(err, ErrExists) {
		return Exists
	}

	if err != nil {
		log.ErrorContext(ctx, "save giveaway", "error", err)

		return Failed
	}

	return Created
}

// Cancel discards the user's unfinished setup from any step.
func (s *Service) Cancel(ctx context.Context, userID int64) Reply {
	log := s.log.With("user_id", userID)

	current, err := s.steps.PendingCommand(ctx, userID)
	if err != nil {
		log.ErrorContext(ctx, "read setup step", "error", err)

		return Reply{Notice: i18n.GiveawayFailed}
	}

	if !inSetup(current) {
		return Reply{Stale: true}
	}

	if r, ok := s.claim(ctx, log, userID, current, ""); !ok {
		return r
	}

	if err := s.drafts.DeleteDraft(ctx, userID); err != nil {
		log.ErrorContext(ctx, "delete giveaway draft", "error", err)
	}

	return Reply{Notice: i18n.GiveawayCancelled}
}

// Summary returns the text that shows the user's draft before Confirm.
func (s *Service) Summary(ctx context.Context, userID int64, lang string) (string, error) {
	d, err := s.drafts.Draft(ctx, userID)
	if err != nil {
		return "", err //nolint:wrapcheck // Store already names the user
	}

	ends := d.EndsAt.In(s.kyiv).Format(dateLayout + " " + timeLayout)

	return s.texts.Text(lang, i18n.GiveawaySummary, d.ChannelTitle, ends, d.Winners, d.ButtonText), nil
}

// Default returns the value a step's default button stands for, or "" if the step has none.
func (s *Service) Default(step, lang string) string {
	switch step {
	case StepEndTime:
		return defaultEndTime
	case StepWinners:
		return strconv.Itoa(minWinners)
	case StepButtonText:
		return s.texts.Text(lang, i18n.GiveawayButton)
	default:
		return ""
	}
}

// claim moves the user from step from to step to. It returns false with a stale Reply when
// another update moved the step first.
func (s *Service) claim(ctx context.Context, log *slog.Logger, userID int64, from, to string) (Reply, bool) {
	moved, err := s.steps.MovePendingCommand(ctx, userID, from, to)
	if err != nil {
		log.ErrorContext(ctx, "move setup step", "error", err)

		return Reply{Notice: i18n.GiveawayFailed}, false
	}

	if !moved {
		return Reply{Stale: true}, false
	}

	return Reply{Next: to}, true
}

// end clears the user's step and draft after the setup failed.
func (s *Service) end(ctx context.Context, userID int64) {
	if err := s.steps.SetPendingCommand(ctx, userID, ""); err != nil {
		s.log.ErrorContext(ctx, "clear setup step", "user_id", userID, "error", err)
	}

	if err := s.drafts.DeleteDraft(ctx, userID); err != nil {
		s.log.ErrorContext(ctx, "delete giveaway draft", "user_id", userID, "error", err)
	}
}

// parseDate parses a YYYY-MM-DD date that is not before today in Kyiv.
func parseDate(value string, kyiv *time.Location, now time.Time) (string, i18n.Key) {
	date, err := time.ParseInLocation(dateLayout, strings.TrimSpace(value), kyiv)
	if err != nil {
		return "", i18n.GiveawayInvalidDate
	}

	if date.Format(dateLayout) < now.In(kyiv).Format(dateLayout) {
		return "", i18n.GiveawayDateInPast
	}

	return date.Format(dateLayout), ""
}

// parseTime parses an HH:MM time on date in Kyiv and returns the moment in UTC, which must
// be after now.
func parseTime(date, value string, kyiv *time.Location, now time.Time) (time.Time, i18n.Key) {
	ends, err := time.ParseInLocation(dateLayout+" "+timeLayout, date+" "+strings.TrimSpace(value), kyiv)
	if err != nil {
		return time.Time{}, i18n.GiveawayInvalidTime
	}

	if !ends.After(now) {
		return time.Time{}, i18n.GiveawayTimeInPast
	}

	return ends.UTC(), ""
}

// parseWinners parses a whole number of winners from minWinners to maxWinners.
func parseWinners(value string) (int, i18n.Key) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < minWinners || n > maxWinners {
		return 0, i18n.GiveawayInvalidWinners
	}

	return n, ""
}

// parseButtonText accepts a non-blank join button text of at most maxButtonTextRune
// characters; Telegram counts characters, not bytes, so Cyrillic gets the full length.
func parseButtonText(value string) (string, i18n.Key) {
	text := strings.TrimSpace(value)
	if text == "" || utf8.RuneCountInString(text) > maxButtonTextRune {
		return "", i18n.GiveawayInvalidButtonText
	}

	return text, ""
}
