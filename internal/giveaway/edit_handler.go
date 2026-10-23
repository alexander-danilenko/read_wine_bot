package giveaway

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/keyboard"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
)

// editPayload starts the start payload of an edit link from the giveaway list.
const editPayload = "edit_"

// EditLinkPrefix starts the message Telegram sends when the user taps an edit link from the
// giveaway list: /start edit_<channel>_<post>.
const EditLinkPrefix = "/start " + editPayload

// fieldPayload starts the start payload of a field link in the field menu. No start payload
// starts another, so each link handler gets only its own taps.
const fieldPayload = "field_"

// FieldLinkPrefix starts the message Telegram sends when the user taps a field link in the
// field menu: /start field_<field>_<giveaway ID>.
const FieldLinkPrefix = "/start " + fieldPayload

// EditCallbackPrefix starts the callback data of every edit button. It is not a prefix of
// SetupCallbackPrefix or JoinCallback, so each handler gets only its own taps.
const EditCallbackPrefix = "giveaway-edit:"

// Edit button actions. Every action but stop carries the giveaway ID, and the default action
// also the field: giveaway-edit:<action>[:<field>]:<giveaway ID>.
const (
	editDefault   = "default"    // saves a field's default value
	editAsk       = "ask"        // asks to confirm the cancel
	editYes       = "yes"        // cancels the giveaway
	editAskFinish = "ask_finish" // asks to confirm finishing now
	editYesFinish = "yes_finish" // finishes the giveaway now
	editNo        = "no"         // declines the cancel or finish and shows the menu
	editPublish   = "publish"    // publishes a cancelled giveaway again
	editMove      = "move"       // asks for the post to move the giveaway to
	editStop      = "stop"       // stops waiting for a forward or a value
)

// editData returns the callback data of an edit button; parts follow the action.
func editData(action string, parts ...string) string {
	return EditCallbackPrefix + strings.Join(append([]string{action}, parts...), ":")
}

// editAction is a parsed edit button tap.
type editAction struct {
	name  string
	field string // set for default
	id    uint   // set for every action but stop
}

// parseEditData parses the callback data of an edit button and reports false for anything
// else, such as a button from an older version of the bot.
func parseEditData(data string) (editAction, bool) {
	parts := strings.Split(strings.TrimPrefix(data, EditCallbackPrefix), ":")

	a := editAction{name: parts[0]}
	if a.name == editStop {
		return a, len(parts) == 1
	}

	rest := parts[1:]
	if a.name == editDefault {
		if len(rest) == 0 || !isField(rest[0]) {
			return editAction{}, false
		}

		a.field, rest = rest[0], rest[1:]
	}

	if len(rest) != 1 {
		return editAction{}, false
	}

	id, err := strconv.ParseUint(rest[0], 10, 0)
	if err != nil {
		return editAction{}, false
	}

	a.id = uint(id)

	return a, true
}

// editHandler translates edit updates into Editor calls and their replies into messages.
type editHandler struct {
	ed     *Editor
	kb     *keyboard.Tracker
	texts  *i18n.Catalog
	log    *slog.Logger
	tapped int // the message whose button was tapped, replaced in place by the next prompt; 0 for none
}

// EditStartHandler returns the handler for /edit in a private chat: it asks for the post of the
// giveaway to change.
func EditStartHandler(
	ed *Editor, texts *i18n.Catalog, users language.Reader, kb *keyboard.Tracker, log *slog.Logger,
) bot.HandlerFunc {
	h := editHandler{ed: ed, kb: kb, texts: texts, log: log}

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := privateMessage(update)
		if msg == nil {
			return
		}

		if err := ed.BeginEdit(ctx, msg.From.ID); err != nil {
			log.ErrorContext(ctx, "start giveaway edit", "user_id", msg.From.ID, "error", err)

			return
		}

		h.prompt(ctx, b, msg.From.ID, language.UserLanguage(ctx, users, log, msg.From), EditStepPost)
	}
}

// ListHandler returns the handler for /list: it sends the user's running giveaways.
func ListHandler(ed *Editor, texts *i18n.Catalog, users language.Reader, log *slog.Logger) bot.HandlerFunc {
	h := editHandler{ed: ed, texts: texts, log: log}

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := privateMessage(update)
		if msg == nil {
			return
		}

		lang := language.UserLanguage(ctx, users, log, msg.From)
		typing(ctx, b, log, msg.From.ID)

		text, err := list(ctx, b, ed, msg.From.ID, lang)
		if err != nil {
			log.ErrorContext(ctx, "list running giveaways", "user_id", msg.From.ID, "error", err)
			text = texts.Text(lang, i18n.EditFailed)
		}

		send(ctx, b, h.log, &bot.SendMessageParams{
			ChatID:             msg.From.ID,
			Text:               text,
			ParseMode:          models.ParseModeHTML,
			LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
		})
	}
}

// list returns the user's running giveaways from List.
func list(ctx context.Context, b *bot.Bot, ed *Editor, userID int64, lang string) (string, error) {
	name, err := botUsername(ctx, b)
	if err != nil {
		return "", err
	}

	return ed.List(ctx, telegramAPI{b: b}, userID, lang, name)
}

// botUsername returns the bot's username, which links to the bot need and only getMe tells.
func botUsername(ctx context.Context, b *bot.Bot) (string, error) {
	me, err := b.GetMe(ctx)
	if err != nil {
		return "", fmt.Errorf("get bot username: %w", err)
	}

	return me.Username, nil
}

// EditLinkHandler returns the handler for the edit links of the giveaway list: it opens the
// field menu of the giveaway on the post the link names.
func EditLinkHandler(
	ed *Editor, texts *i18n.Catalog, users language.Reader, kb *keyboard.Tracker, log *slog.Logger,
) bot.HandlerFunc {
	h := editHandler{ed: ed, kb: kb, texts: texts, log: log}

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := privateMessage(update)
		if msg == nil {
			return
		}

		dropLinkTap(ctx, b, log, msg)

		lang := language.UserLanguage(ctx, users, log, msg.From)
		typing(ctx, b, log, msg.From.ID)

		chatID, messageID, ok := parseEditLink(msg.Text)
		if !ok {
			h.reply(ctx, b, msg.From.ID, lang, Reply{Notice: i18n.EditNotFound})

			return
		}

		h.reply(ctx, b, msg.From.ID, lang, ed.OpenPost(ctx, telegramAPI{b: b}, msg.From.ID, chatID, messageID))
	}
}

// FieldLinkHandler returns the handler for the field links of the field menu: it asks for a new
// value of the field the link names.
func FieldLinkHandler(
	ed *Editor, texts *i18n.Catalog, users language.Reader, kb *keyboard.Tracker, log *slog.Logger,
) bot.HandlerFunc {
	h := editHandler{ed: ed, kb: kb, texts: texts, log: log}

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := privateMessage(update)
		if msg == nil {
			return
		}

		dropLinkTap(ctx, b, log, msg)

		lang := language.UserLanguage(ctx, users, log, msg.From)
		typing(ctx, b, log, msg.From.ID)

		r := Reply{Stale: true}
		if id, field, ok := parseFieldLink(msg.Text); ok {
			r = ed.Field(ctx, telegramAPI{b: b}, msg.From.ID, id, field)
		}

		// A link answers a message, which shows no popup, so a link that no longer applies
		// gets a notice.
		if r.Stale {
			r = Reply{Notice: i18n.EditNotFound}
		}

		h.reply(ctx, b, msg.From.ID, lang, r)
	}
}

// parseFieldLink parses /start field_<field>_<giveaway ID> into the giveaway ID and the field.
// Field names hold underscores, so the ID follows the last one.
func parseFieldLink(text string) (uint, string, bool) {
	field, raw, found := cutLast(strings.TrimPrefix(strings.TrimSpace(text), FieldLinkPrefix), "_")
	if !found || !positive(raw) {
		return 0, "", false
	}

	id, err := strconv.ParseUint(raw, 10, 0)

	return uint(id), field, err == nil
}

// cutLast slices s around the last sep, like strings.Cut around the first.
func cutLast(s, sep string) (string, string, bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}

	return s[:i], s[i+len(sep):], true
}

// parseEditLink parses /start edit_<channel>_<post> into the post's chat and message IDs.
func parseEditLink(text string) (int64, int64, bool) {
	return parsePostPayload(text, EditLinkPrefix)
}

// parsePostPayload parses a start link message, prefix followed by <channel>_<post>, into the
// post's chat and message IDs.
func parsePostPayload(text, prefix string) (int64, int64, bool) {
	channel, post, found := strings.Cut(strings.TrimPrefix(strings.TrimSpace(text), prefix), "_")
	if !found || !positive(channel) || !positive(post) {
		return 0, 0, false
	}

	chatID, errChat := strconv.ParseInt("-100"+channel, 10, 64)
	messageID, errPost := strconv.ParseInt(post, 10, 64)

	return chatID, messageID, errChat == nil && errPost == nil
}

// message hands a private message to the edit step the user is at: the post and move steps
// take a forwarded channel post or its link, a field step takes text, and any other input
// asks again.
func (h editHandler) message(ctx context.Context, b *bot.Bot, msg *models.Message, step, lang string) {
	chatID, messageID, isPost := postRef(ctx, b, h.log, msg)

	tg := telegramAPI{b: b}
	what, _, _ := parseEditStep(step)

	switch {
	case what == moveStep && isPost:
		h.reply(ctx, b, msg.From.ID, lang, h.ed.Move(ctx, tg, tg, msg.From.ID, chatID, messageID))
	case what == moveStep:
		h.prompt(ctx, b, msg.From.ID, lang, step)
	case step == EditStepPost && isPost:
		h.reply(
			ctx,
			b,
			msg.From.ID,
			lang,
			h.ed.ForwardEdit(ctx, tg, msg.From.ID, chatID, messageID),
		)
	case step == EditStepPost || isPost:
		h.prompt(ctx, b, msg.From.ID, lang, step)
	default:
		h.reply(ctx, b, msg.From.ID, lang, h.ed.Value(ctx, tg, msg.From.ID, step, msg.Text))
	}
}

// EditCallbackHandler returns the handler for the edit buttons. The next prompt replaces the
// tapped message; a tap that ends in a notice deletes it, and a button whose action no longer
// applies also gets an "outdated" popup.
func EditCallbackHandler(
	ed *Editor, texts *i18n.Catalog, users language.Reader, kb *keyboard.Tracker, log *slog.Logger,
) bot.HandlerFunc {
	h := editHandler{ed: ed, kb: kb, texts: texts, log: log}

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		query := update.CallbackQuery
		if query == nil {
			return
		}

		userID := query.From.ID
		lang := language.UserLanguage(ctx, users, log, &query.From)

		r := Reply{Stale: true}
		if a, ok := parseEditData(query.Data); ok {
			r = h.act(ctx, b, userID, lang, a)
		}

		popup := ""
		if r.Stale {
			popup = texts.Text(lang, i18n.GiveawayOutdated)
		}

		// Telegram shows a spinner on the button until the query is answered.
		_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: popup})
		if err != nil {
			log.ErrorContext(ctx, "answer edit button", "user_id", userID, "error", err)
		}

		tapped := inPlace(r, query.Message)
		if tapped == 0 {
			kb.Discard(ctx, b, query.Message)
		}

		// Edit buttons are sent only in private chats, where the chat ID equals the user ID.
		h.at(tapped).reply(ctx, b, userID, lang, r)
	}
}

// act runs the Editor call a tap asks for.
func (h editHandler) act(ctx context.Context, b *bot.Bot, userID int64, lang string, a editAction) Reply {
	tg := telegramAPI{b: b}

	switch a.name {
	case editDefault:
		return h.ed.Value(ctx, tg, userID, editStep(a.field, a.id), h.ed.Default(a.field, lang))
	case editAsk, editYes, editAskFinish, editYesFinish, editNo:
		return h.confirm(ctx, tg, userID, a)
	case editPublish:
		return h.ed.Restore(ctx, tg, userID, a.id)
	case editMove:
		return h.ed.AskMove(ctx, tg, userID, a.id)
	case editStop:
		return h.ed.Stop(ctx, userID)
	default:
		return Reply{Stale: true}
	}
}

// confirm runs the Editor call of a tap that asks for, accepts or declines a cancel or finish.
func (h editHandler) confirm(ctx context.Context, tg telegramAPI, userID int64, a editAction) Reply {
	switch a.name {
	case editAsk:
		return h.ed.AskCancel(ctx, tg, userID, a.id)
	case editYes:
		return h.ed.Cancel(ctx, tg, tg, userID, a.id)
	case editAskFinish:
		return h.ed.AskFinish(ctx, tg, userID, a.id)
	case editYesFinish:
		return h.ed.FinishNow(ctx, tg, userID, a.id)
	default:
		return h.ed.ShowMenu(ctx, tg, userID, a.id)
	}
}

// reply deletes the previous message with buttons, then sends the notice of r and the prompt
// of its next step. A stale r sends nothing and keeps the message with buttons.
func (h editHandler) reply(ctx context.Context, b *bot.Bot, userID int64, lang string, r Reply) {
	if r.Stale {
		return
	}

	if h.tapped == 0 {
		h.kb.Clear(ctx, b, userID)
	}

	if r.Notice != "" {
		notify(ctx, b, h.log, userID, h.texts.Text(lang, r.Notice))
	}

	if r.Next != "" {
		h.prompt(ctx, b, userID, lang, r.Next)
	}
}

// prompt shows step: the forward request, the field menu, a cancel or finish confirmation, or the
// question for a field with its default button. Waiting steps get a Cancel button.
func (h editHandler) prompt(ctx context.Context, b *bot.Bot, userID int64, lang, step string) {
	button := func(key i18n.Key, data string) models.InlineKeyboardButton {
		return models.InlineKeyboardButton{Text: h.texts.Text(lang, key), CallbackData: data}
	}
	stop := []models.InlineKeyboardButton{button(i18n.GiveawayCancel, editData(editStop))}

	what, id, ok := parseEditStep(step)
	if !ok {
		h.ask(ctx, b, userID, h.texts.Text(lang, i18n.EditPrompt), [][]models.InlineKeyboardButton{stop})

		return
	}

	rawID := strconv.FormatUint(uint64(id), 10)

	switch what {
	case menuStep:
		h.menu(ctx, b, userID, lang, id)
	case askCancelStep:
		h.ask(ctx, b, userID, h.texts.Text(lang, i18n.EditAskCancel), [][]models.InlineKeyboardButton{{
			button(i18n.EditYes, editData(editYes, rawID)),
			button(i18n.EditNo, editData(editNo, rawID)),
		}})
	case moveStep:
		h.ask(ctx, b, userID, h.texts.Text(lang, i18n.EditMovePrompt), [][]models.InlineKeyboardButton{stop})
	case askFinishStep:
		h.ask(ctx, b, userID, h.texts.Text(lang, i18n.EditAskFinish), [][]models.InlineKeyboardButton{{
			button(i18n.EditYesFinish, editData(editYesFinish, rawID)),
			button(i18n.EditNo, editData(editNo, rawID)),
		}})
	default:
		var rows [][]models.InlineKeyboardButton
		if value := h.ed.Default(what, lang); value != "" {
			rows = append(
				rows,
				[]models.InlineKeyboardButton{{Text: value, CallbackData: editData(editDefault, what, rawID)}},
			)
		}

		h.ask(ctx, b, userID, h.texts.Text(lang, promptKey(stepPrefix+what)), append(rows, stop))
	}
}

// menu sends the HTML field menu of giveaway id, with a link per field in the text, and the
// buttons Move, then Finish now and Cancel for a published giveaway or Publish for a cancelled
// one.
func (h editHandler) menu(ctx context.Context, b *bot.Bot, userID int64, lang string, id uint) {
	name, err := botUsername(ctx, b)

	var (
		text      string
		published bool
	)
	if err == nil {
		text, published, err = h.ed.Menu(ctx, id, lang, name)
	}

	if err != nil {
		h.log.ErrorContext(ctx, "build giveaway edit menu", "user_id", userID, "giveaway_id", id, "error", err)
		send(ctx, b, h.log, &bot.SendMessageParams{ChatID: userID, Text: h.texts.Text(lang, i18n.EditFailed)})

		return
	}

	rawID := strconv.FormatUint(uint64(id), 10)
	row := func(key i18n.Key, action string) []models.InlineKeyboardButton {
		return []models.InlineKeyboardButton{{Text: h.texts.Text(lang, key), CallbackData: editData(action, rawID)}}
	}

	tail := [][]models.InlineKeyboardButton{
		row(i18n.EditMoveGiveaway, editMove),
		row(i18n.EditPublishGiveaway, editPublish),
	}
	if published {
		tail = [][]models.InlineKeyboardButton{
			row(i18n.EditMoveGiveaway, editMove),
			row(i18n.EditFinishGiveaway, editAskFinish),
			row(i18n.EditCancelGiveaway, editAsk),
		}
	}

	showHTML(ctx, b, h.kb, h.tapped, userID, text, tail)
}

// ask shows text with an inline keyboard of rows in place of the previous one.
func (h editHandler) ask(
	ctx context.Context,
	b *bot.Bot,
	userID int64,
	text string,
	rows [][]models.InlineKeyboardButton,
) {
	show(ctx, b, h.kb, h.tapped, userID, text, rows)
}

// at returns h with the prompt replacing message tapped; 0 sends a new message instead.
func (h editHandler) at(tapped int) editHandler {
	h.tapped = tapped

	return h
}
