package giveaway

import (
	"context"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
)

// SetupCallbackPrefix starts the callback data of every setup button. It must not be a
// prefix of JoinCallback, or the setup handler would take the join taps.
const SetupCallbackPrefix = "giveaway-setup:"

// Setup button callback data; a default button appends the step it belongs to.
const (
	cancelData  = SetupCallbackPrefix + "cancel"
	confirmData = SetupCallbackPrefix + "confirm"
	defaultData = SetupCallbackPrefix + "default:"
)

// Users reads a user's saved language and setup step. *user.Store implements it.
type Users interface {
	language.Reader
	PendingCommand(ctx context.Context, telegramID int64) (string, error)
}

// promptKey returns the question a setup step asks; the confirm step shows the summary instead.
func promptKey(step string) i18n.Key {
	switch step {
	case StepEndDate:
		return i18n.GiveawayAskEndDate
	case StepEndTime:
		return i18n.GiveawayAskEndTime
	case StepWinners:
		return i18n.GiveawayAskWinners
	case StepButtonText:
		return i18n.GiveawayAskButtonText
	default:
		return i18n.GiveawayPrompt
	}
}

// setupHandler translates setup updates into Service calls and their replies into messages.
type setupHandler struct {
	svc   *Service
	texts *i18n.Catalog
	log   *slog.Logger
}

// StartHandler returns the handler for the giveaway menu button in a private chat: it starts
// a new setup and asks for the post.
func StartHandler(svc *Service, texts *i18n.Catalog, users Users, log *slog.Logger) bot.HandlerFunc {
	h := setupHandler{svc: svc, texts: texts, log: log}

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := privateMessage(update)
		if msg == nil {
			return
		}

		if err := svc.Begin(ctx, msg.From.ID); err != nil {
			log.ErrorContext(ctx, "start giveaway setup", "user_id", msg.From.ID, "error", err)

			return
		}

		h.prompt(ctx, b, msg.From.ID, language.UserLanguage(ctx, users, log, msg.From), StepPost)
	}
}

// MessageHandler returns the handler for private messages no other handler matched. It acts
// only while the sender is in a giveaway setup or edit: the post step takes a forwarded
// channel post, the value steps take text, and any other input asks the current step again.
func MessageHandler(svc *Service, ed *Editor, texts *i18n.Catalog, users Users, log *slog.Logger) bot.HandlerFunc {
	h := setupHandler{svc: svc, texts: texts, log: log}
	edit := editHandler{ed: ed, texts: texts, log: log}

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := privateMessage(update)
		if msg == nil {
			return
		}

		step, err := users.PendingCommand(ctx, msg.From.ID)
		if err != nil {
			log.ErrorContext(ctx, "read setup step", "user_id", msg.From.ID, "error", err)

			return
		}

		if !inSetup(step) && !inEdit(step) {
			return
		}

		lang := language.UserLanguage(ctx, users, log, msg.From)
		if inSetup(step) {
			h.step(ctx, b, msg, step, lang)
		} else {
			edit.message(ctx, b, msg, step, lang)
		}
	}
}

// step hands the message to the setup step the user is at.
func (h setupHandler) step(ctx context.Context, b *bot.Bot, msg *models.Message, step, lang string) {
	chatID, messageID, isPost := postRef(ctx, b, h.log, msg)

	switch {
	case step == StepPost && isPost:
		tg := telegramAPI{b: b}
		r := h.svc.Forward(ctx, tg, msg.From.ID, chatID, messageID, lang)
		h.reply(ctx, b, msg.From.ID, lang, r)
	case step == StepPost || step == StepConfirm || isPost:
		h.prompt(ctx, b, msg.From.ID, lang, step)
	default:
		h.reply(ctx, b, msg.From.ID, lang, h.svc.Apply(ctx, msg.From.ID, step, msg.Text))
	}
}

// SetupCallbackHandler returns the handler for the default, Cancel and Confirm buttons. A
// button whose step has passed gets an "outdated" popup and loses its buttons.
func SetupCallbackHandler(svc *Service, texts *i18n.Catalog, users Users, log *slog.Logger) bot.HandlerFunc {
	h := setupHandler{svc: svc, texts: texts, log: log}

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		query := update.CallbackQuery
		if query == nil {
			return
		}

		userID := query.From.ID
		lang := language.UserLanguage(ctx, users, log, &query.From)

		var r Reply

		switch {
		case query.Data == cancelData:
			r = svc.Cancel(ctx, userID)
		case query.Data == confirmData:
			r = svc.Confirm(ctx, telegramAPI{b: b}, userID)
		case strings.HasPrefix(query.Data, defaultData):
			step := strings.TrimPrefix(query.Data, defaultData)
			r = svc.Apply(ctx, userID, step, svc.Default(step, lang))
		default:
			r = Reply{Stale: true}
		}

		popup := ""
		if r.Stale {
			popup = texts.Text(lang, i18n.GiveawayOutdated)
		}

		// Telegram shows a spinner on the button until the query is answered.
		_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: popup})
		if err != nil {
			log.ErrorContext(ctx, "answer setup button", "user_id", userID, "error", err)
		}

		if r.Stale {
			removeButtons(ctx, b, log, query.Message)

			return
		}

		// Setup buttons are sent only in private chats, where the chat ID equals the user ID.
		h.reply(ctx, b, userID, lang, r)
	}
}

// reply sends the notice of r, then the prompt of its next step.
func (h setupHandler) reply(ctx context.Context, b *bot.Bot, userID int64, lang string, r Reply) {
	if r.Notice != "" {
		send(ctx, b, h.log, &bot.SendMessageParams{ChatID: userID, Text: h.texts.Text(lang, r.Notice)})
	}

	if r.Next != "" {
		h.prompt(ctx, b, userID, lang, r.Next)
	}
}

// prompt asks for step, with its default button if it has one and a Cancel button. The
// confirm step shows the summary with Confirm and Cancel.
func (h setupHandler) prompt(ctx context.Context, b *bot.Bot, userID int64, lang, step string) {
	cancel := models.InlineKeyboardButton{Text: h.texts.Text(lang, i18n.GiveawayCancel), CallbackData: cancelData}

	if step == StepConfirm {
		summary, err := h.svc.Summary(ctx, userID, lang)
		if err != nil {
			h.log.ErrorContext(ctx, "build giveaway summary", "user_id", userID, "error", err)
			send(ctx, b, h.log, &bot.SendMessageParams{ChatID: userID, Text: h.texts.Text(lang, i18n.GiveawayFailed)})

			return
		}

		confirm := models.InlineKeyboardButton{
			Text:         h.texts.Text(lang, i18n.GiveawayConfirm),
			CallbackData: confirmData,
		}
		send(ctx, b, h.log, &bot.SendMessageParams{
			ChatID: userID,
			Text:   summary,
			ReplyMarkup: &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{{confirm, cancel}},
			},
		})

		return
	}

	var rows [][]models.InlineKeyboardButton
	if value := h.svc.Default(step, lang); value != "" {
		rows = append(rows, []models.InlineKeyboardButton{{Text: value, CallbackData: defaultData + step}})
	}

	rows = append(rows, []models.InlineKeyboardButton{cancel})

	send(ctx, b, h.log, &bot.SendMessageParams{
		ChatID:      userID,
		Text:        h.texts.Text(lang, promptKey(step)),
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

// send sends a message and logs a failure; the user then sees no reply and can try again.
func send(ctx context.Context, b *bot.Bot, log *slog.Logger, params *bot.SendMessageParams) {
	if _, err := b.SendMessage(ctx, params); err != nil {
		log.ErrorContext(ctx, "send giveaway message", "chat_id", params.ChatID, "error", err)
	}
}

// privateMessage returns the update's message when a user sent it in a private chat, and nil
// for anything else.
func privateMessage(update *models.Update) *models.Message {
	msg := update.Message
	if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
		return nil
	}

	return msg
}

// postRef returns the channel post a message points to: the post it was forwarded from, or
// the post its text links to. A link costs a getChat call, which turns the channel in the link
// into its chat ID and confirms the chat is a channel.
func postRef(ctx context.Context, b *bot.Bot, log *slog.Logger, msg *models.Message) (int64, int64, bool) {
	if msg.ForwardOrigin != nil && msg.ForwardOrigin.MessageOriginChannel != nil {
		origin := msg.ForwardOrigin.MessageOriginChannel

		return origin.Chat.ID, int64(origin.MessageID), true
	}

	chat, messageID, ok := parsePostLink(msg.Text)
	if !ok {
		return 0, 0, false
	}

	info, err := b.GetChat(ctx, &bot.GetChatParams{ChatID: chat})
	if err != nil {
		log.WarnContext(ctx, "resolve post link", "user_id", msg.From.ID, "chat", chat, "error", err)

		return 0, 0, false
	}

	return info.ID, messageID, info.Type == models.ChatTypeChannel
}

// parsePostLink parses a channel post link and returns the chat in the form getChat takes: a
// public link https://t.me/<username>/<post> gives "@<username>", a private link
// https://t.me/c/<channel>/<post> gives the chat ID, which the link writes without its -100.
func parsePostLink(text string) (string, int64, bool) {
	u, err := url.Parse(strings.TrimSpace(text))
	if err != nil || u.Host != "t.me" {
		return "", 0, false
	}

	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")

	var chat string

	switch {
	case len(parts) == 3 && parts[0] == "c" && positive(parts[1]):
		chat = "-100" + parts[1]
	case len(parts) == 2 && isUsername(parts[0]):
		chat = "@" + parts[0]
	default:
		return "", 0, false
	}

	post := parts[len(parts)-1]
	if !positive(post) {
		return "", 0, false
	}

	messageID, err := strconv.ParseInt(post, 10, 64)

	return chat, messageID, err == nil
}

// positive reports whether s is a number above zero written in plain digits, with no sign.
func positive(s string) bool {
	n, err := strconv.ParseUint(s, 10, 63)

	return err == nil && n > 0
}

// isUsername reports whether s can be a Telegram username: 4-32 Latin letters, digits and
// underscores, starting with a letter.
func isUsername(s string) bool {
	if len(s) < 4 || len(s) > 32 || !isLetter(rune(s[0])) {
		return false
	}

	for _, r := range s {
		if !isLetter(r) && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}

	return true
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// removeButtons removes the inline buttons from the message an outdated button belongs to.
func removeButtons(ctx context.Context, b *bot.Bot, log *slog.Logger, m models.MaybeInaccessibleMessage) {
	if m.Message == nil {
		return
	}

	_, err := b.EditMessageReplyMarkup(
		ctx,
		&bot.EditMessageReplyMarkupParams{ChatID: m.Message.Chat.ID, MessageID: m.Message.ID},
	)
	if err = ignoreNotModified(err); err != nil {
		log.WarnContext(ctx, "remove outdated buttons", "chat_id", m.Message.Chat.ID, "error", err)
	}
}
