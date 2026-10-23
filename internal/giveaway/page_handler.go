package giveaway

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/keyboard"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
)

// FinishedCallbackPrefix starts the callback data of the page buttons of /list_finished:
// giveaway-finished:<page>. No callback prefix starts another, so each handler gets only its
// own taps.
const FinishedCallbackPrefix = "giveaway-finished:"

// renderPage returns the HTML text of page, from 1, of a list for the user, with links to the
// bot named botUsername, and whether a next page exists.
type renderPage func(
	ctx context.Context, tg Telegram, userID int64, page int, lang, botUsername string,
) (string, bool, error)

// pager shows a list page by page, with buttons to the previous and the next page.
type pager struct {
	prefix string // starts the callback data of the page buttons
	what   string // names the list in the log
	render renderPage
	users  language.Reader
	kb     *keyboard.Tracker
	texts  *i18n.Catalog
	log    *slog.Logger
}

// finishedPager returns the pager of /list_finished: the user's finished giveaways.
func finishedPager(
	ed *Editor, texts *i18n.Catalog, users language.Reader, kb *keyboard.Tracker, log *slog.Logger,
) pager {
	render := func(ctx context.Context, _ Telegram, userID int64, page int, lang, name string) (string, bool, error) {
		return ed.Finished(ctx, userID, page, lang, name)
	}

	return pager{
		prefix: FinishedCallbackPrefix, what: "finished giveaways", render: render,
		users: users, kb: kb, texts: texts, log: log,
	}
}

// ListFinishedHandler returns the handler for /list_finished: it sends the first page of the
// user's finished giveaways.
func ListFinishedHandler(
	ed *Editor, texts *i18n.Catalog, users language.Reader, kb *keyboard.Tracker, log *slog.Logger,
) bot.HandlerFunc {
	return finishedPager(ed, texts, users, kb, log).command()
}

// FinishedCallbackHandler returns the handler for the page buttons of /list_finished.
func FinishedCallbackHandler(
	ed *Editor, texts *i18n.Catalog, users language.Reader, kb *keyboard.Tracker, log *slog.Logger,
) bot.HandlerFunc {
	return finishedPager(ed, texts, users, kb, log).callback()
}

// command returns the handler of the list's command: it sends the first page.
func (p pager) command() bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := privateMessage(update)
		if msg == nil {
			return
		}

		lang := language.UserLanguage(ctx, p.users, p.log, msg.From)
		typing(ctx, b, p.log, msg.From.ID)
		p.show(ctx, b, msg.From.ID, lang, 1, 0)
	}
}

// callback returns the handler of the list's page buttons: the page replaces the tapped
// message.
func (p pager) callback() bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		query := update.CallbackQuery
		if query == nil {
			return
		}

		lang := language.UserLanguage(ctx, p.users, p.log, &query.From)
		page, ok := parsePage(query.Data, p.prefix)

		popup := ""
		if !ok || query.Message.Message == nil {
			popup = p.texts.Text(lang, i18n.GiveawayOutdated)
		}

		// Telegram shows a spinner on the button until the query is answered.
		_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: popup})
		if err != nil {
			p.log.ErrorContext(ctx, "answer page button", "list", p.what, "user_id", query.From.ID, "error", err)
		}

		if popup == "" {
			// Page buttons are sent only in private chats, where the chat ID equals the user ID.
			p.show(ctx, b, query.From.ID, lang, page, query.Message.Message.ID)
		}
	}
}

// show shows page of the list with its page buttons in place of message tapped, or as a new
// message when tapped is 0. A page without buttons goes as a plain notice.
func (p pager) show(ctx context.Context, b *bot.Bot, userID int64, lang string, page, tapped int) {
	name, err := botUsername(ctx, b)

	var (
		text string
		more bool
	)
	if err == nil {
		text, more, err = p.render(ctx, telegramAPI{b: b}, userID, page, lang, name)
	}

	if err != nil {
		p.log.ErrorContext(ctx, "list giveaways", "list", p.what, "user_id", userID, "page", page, "error", err)
		send(ctx, b, p.log, &bot.SendMessageParams{ChatID: userID, Text: p.texts.Text(lang, i18n.EditFailed)})

		return
	}

	var row []models.InlineKeyboardButton
	if page > 1 {
		row = append(row, p.button(lang, i18n.PagePrev, page-1))
	}

	if more {
		row = append(row, p.button(lang, i18n.PageNext, page+1))
	}

	if row == nil && tapped == 0 {
		send(ctx, b, p.log, &bot.SendMessageParams{
			ChatID:             userID,
			Text:               text,
			ParseMode:          models.ParseModeHTML,
			LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
		})

		return
	}

	var rows [][]models.InlineKeyboardButton
	if row != nil {
		rows = append(rows, row)
	}

	showHTML(ctx, b, p.kb, tapped, userID, text, rows)
}

// button returns the page button labelled key that shows page.
func (p pager) button(lang string, key i18n.Key, page int) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: p.texts.Text(lang, key, page), CallbackData: p.prefix + strconv.Itoa(page)}
}

// parsePage returns the page of a page button's callback data under prefix, and false for
// anything else.
func parsePage(data, prefix string) (int, bool) {
	raw, found := strings.CutPrefix(data, prefix)
	if !found || !positive(raw) {
		return 0, false
	}

	page, err := strconv.ParseUint(raw, 10, 31)
	if err != nil {
		return 0, false
	}

	return int(page), true
}
