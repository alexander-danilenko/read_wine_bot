package giveaway

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
)

// EditCallbackPrefix starts the callback data of every edit button. It is not a prefix of
// SetupCallbackPrefix or JoinCallback, so each handler gets only its own taps.
const EditCallbackPrefix = "giveaway-edit:"

// Edit button actions. Every action but stop carries the giveaway ID, and the field and
// default actions also the field: giveaway-edit:<action>[:<field>]:<giveaway ID>.
const (
	editField   = "field"   // asks for a field value
	editDefault = "default" // saves a field's default value
	editAsk     = "ask"     // asks to confirm the cancel
	editYes     = "yes"     // cancels the giveaway
	editNo      = "no"      // declines the cancel and shows the menu
	editPublish = "publish" // publishes a cancelled giveaway again
	editStop    = "stop"    // stops waiting for a forward or a value
)

// editData returns the callback data of an edit button; parts follow the action.
func editData(action string, parts ...string) string {
	return EditCallbackPrefix + strings.Join(append([]string{action}, parts...), ":")
}

// editAction is a parsed edit button tap.
type editAction struct {
	name  string
	field string // set for field and default
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
	if a.name == editField || a.name == editDefault {
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
	ed    *Editor
	texts *i18n.Catalog
	log   *slog.Logger
}

// EditStartHandler returns the handler for the edit menu button in a private chat: it asks
// for the post of the giveaway to change.
func EditStartHandler(ed *Editor, texts *i18n.Catalog, users language.Reader, log *slog.Logger) bot.HandlerFunc {
	h := editHandler{ed: ed, texts: texts, log: log}

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

// ListHandler returns the handler for the list menu button: it sends the user's running
// giveaways.
func ListHandler(ed *Editor, texts *i18n.Catalog, users language.Reader, log *slog.Logger) bot.HandlerFunc {
	h := editHandler{ed: ed, texts: texts, log: log}

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := privateMessage(update)
		if msg == nil {
			return
		}

		lang := language.UserLanguage(ctx, users, log, msg.From)

		text, err := ed.List(ctx, msg.From.ID, lang)
		if err != nil {
			log.ErrorContext(ctx, "list running giveaways", "user_id", msg.From.ID, "error", err)
			text = texts.Text(lang, i18n.EditFailed)
		}

		send(ctx, b, h.log, &bot.SendMessageParams{ChatID: msg.From.ID, Text: text})
	}
}

// message hands a private message to the edit step the user is at: the post step takes a
// forwarded channel post, a field step takes text, and any other input asks again.
func (h editHandler) message(ctx context.Context, b *bot.Bot, msg *models.Message, step, lang string) {
	origin := channelOrigin(msg)

	tg := telegramAPI{b: b}

	switch {
	case step == EditStepPost && origin != nil:
		h.reply(
			ctx,
			b,
			msg.From.ID,
			lang,
			h.ed.ForwardEdit(ctx, tg, msg.From.ID, origin.Chat.ID, int64(origin.MessageID)),
		)
	case step == EditStepPost || origin != nil:
		h.prompt(ctx, b, msg.From.ID, lang, step)
	default:
		h.reply(ctx, b, msg.From.ID, lang, h.ed.Value(ctx, tg, msg.From.ID, step, msg.Text))
	}
}

// EditCallbackHandler returns the handler for the edit buttons. A button whose action no
// longer applies gets an "outdated" popup and loses its buttons.
func EditCallbackHandler(ed *Editor, texts *i18n.Catalog, users language.Reader, log *slog.Logger) bot.HandlerFunc {
	h := editHandler{ed: ed, texts: texts, log: log}

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

		if r.Stale {
			removeButtons(ctx, b, log, query.Message)

			return
		}

		// Edit buttons are sent only in private chats, where the chat ID equals the user ID.
		h.reply(ctx, b, userID, lang, r)
	}
}

// act runs the Editor call a tap asks for.
func (h editHandler) act(ctx context.Context, b *bot.Bot, userID int64, lang string, a editAction) Reply {
	tg := telegramAPI{b: b}

	switch a.name {
	case editField:
		return h.ed.Field(ctx, tg, userID, a.id, a.field)
	case editDefault:
		return h.ed.Value(ctx, tg, userID, editStep(a.field, a.id), h.ed.Default(a.field, lang))
	case editAsk:
		return h.ed.AskCancel(ctx, tg, userID, a.id)
	case editYes:
		return h.ed.Cancel(ctx, tg, tg, userID, a.id)
	case editNo:
		return h.ed.ShowMenu(ctx, tg, userID, a.id)
	case editPublish:
		return h.ed.Restore(ctx, tg, userID, a.id)
	case editStop:
		return h.ed.Stop(ctx, userID)
	default:
		return Reply{Stale: true}
	}
}

// reply sends the notice of r, then the prompt of its next step.
func (h editHandler) reply(ctx context.Context, b *bot.Bot, userID int64, lang string, r Reply) {
	if r.Notice != "" {
		send(ctx, b, h.log, &bot.SendMessageParams{ChatID: userID, Text: h.texts.Text(lang, r.Notice)})
	}

	if r.Next != "" {
		h.prompt(ctx, b, userID, lang, r.Next)
	}
}

// prompt shows step: the forward request, the field menu, the cancel confirmation, or the
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

// menu sends the field menu of giveaway id: a button per field, then Cancel for a published
// giveaway or Publish for a cancelled one.
func (h editHandler) menu(ctx context.Context, b *bot.Bot, userID int64, lang string, id uint) {
	text, published, err := h.ed.Menu(ctx, id, lang)
	if err != nil {
		h.log.ErrorContext(ctx, "build giveaway edit menu", "user_id", userID, "giveaway_id", id, "error", err)
		send(ctx, b, h.log, &bot.SendMessageParams{ChatID: userID, Text: h.texts.Text(lang, i18n.EditFailed)})

		return
	}

	rawID := strconv.FormatUint(uint64(id), 10)
	rows := make([][]models.InlineKeyboardButton, 0, len(menuFields)+1)

	for _, f := range menuFields {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text: h.texts.Text(lang, f.label), CallbackData: editData(editField, f.field, rawID),
		}})
	}

	last := models.InlineKeyboardButton{
		Text: h.texts.Text(lang, i18n.EditPublishGiveaway), CallbackData: editData(editPublish, rawID),
	}
	if published {
		last = models.InlineKeyboardButton{
			Text: h.texts.Text(lang, i18n.EditCancelGiveaway), CallbackData: editData(editAsk, rawID),
		}
	}

	h.ask(ctx, b, userID, text, append(rows, []models.InlineKeyboardButton{last}))
}

// menuFields are the field buttons of the menu, in order.
var menuFields = []struct { //nolint:gochecknoglobals // read-only table of the menu buttons
	field string
	label i18n.Key
}{
	{FieldEndDate, i18n.EditFieldEndDate},
	{FieldEndTime, i18n.EditFieldEndTime},
	{FieldWinners, i18n.EditFieldWinners},
	{FieldButtonText, i18n.EditFieldButtonText},
}

// ask sends text with an inline keyboard of rows.
func (h editHandler) ask(
	ctx context.Context,
	b *bot.Bot,
	userID int64,
	text string,
	rows [][]models.InlineKeyboardButton,
) {
	send(ctx, b, h.log, &bot.SendMessageParams{
		ChatID:      userID,
		Text:        text,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}
