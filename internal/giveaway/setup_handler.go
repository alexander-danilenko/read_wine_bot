package giveaway

import (
	"context"
	"log/slog"
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
		msg := update.Message
		if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
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
// only while the sender is in a giveaway setup: the post step takes a forwarded channel post,
// the value steps take text, and any other input asks the current step again.
func MessageHandler(svc *Service, texts *i18n.Catalog, users Users, log *slog.Logger) bot.HandlerFunc {
	h := setupHandler{svc: svc, texts: texts, log: log}

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
			return
		}

		step, err := users.PendingCommand(ctx, msg.From.ID)
		if err != nil {
			log.ErrorContext(ctx, "read setup step", "user_id", msg.From.ID, "error", err)

			return
		}

		if !inSetup(step) {
			return
		}

		h.step(ctx, b, msg, step, language.UserLanguage(ctx, users, log, msg.From))
	}
}

// step hands the message to the setup step the user is at.
func (h setupHandler) step(ctx context.Context, b *bot.Bot, msg *models.Message, step, lang string) {
	var origin *models.MessageOriginChannel
	if msg.ForwardOrigin != nil {
		origin = msg.ForwardOrigin.MessageOriginChannel
	}

	switch {
	case step == StepPost && origin != nil:
		tg := telegramAPI{b: b}
		r := h.svc.Forward(ctx, tg, msg.From.ID, origin.Chat.ID, int64(origin.MessageID), lang)
		h.reply(ctx, b, msg.From.ID, lang, r)
	case step == StepPost || step == StepConfirm || origin != nil:
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
			h.removeButtons(ctx, b, query.Message)

			return
		}

		// Setup buttons are sent only in private chats, where the chat ID equals the user ID.
		h.reply(ctx, b, userID, lang, r)
	}
}

// reply sends the notice of r, then the prompt of its next step.
func (h setupHandler) reply(ctx context.Context, b *bot.Bot, userID int64, lang string, r Reply) {
	if r.Notice != "" {
		h.send(ctx, b, &bot.SendMessageParams{ChatID: userID, Text: h.texts.Text(lang, r.Notice)})
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
			h.send(ctx, b, &bot.SendMessageParams{ChatID: userID, Text: h.texts.Text(lang, i18n.GiveawayFailed)})

			return
		}

		confirm := models.InlineKeyboardButton{
			Text:         h.texts.Text(lang, i18n.GiveawayConfirm),
			CallbackData: confirmData,
		}
		h.send(ctx, b, &bot.SendMessageParams{
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

	h.send(ctx, b, &bot.SendMessageParams{
		ChatID:      userID,
		Text:        h.texts.Text(lang, promptKey(step)),
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
}

func (h setupHandler) send(ctx context.Context, b *bot.Bot, params *bot.SendMessageParams) {
	if _, err := b.SendMessage(ctx, params); err != nil {
		h.log.ErrorContext(ctx, "send giveaway setup message", "chat_id", params.ChatID, "error", err)
	}
}

// removeButtons removes the inline buttons from the message an outdated button belongs to.
func (h setupHandler) removeButtons(ctx context.Context, b *bot.Bot, m models.MaybeInaccessibleMessage) {
	if m.Message == nil {
		return
	}

	_, err := b.EditMessageReplyMarkup(
		ctx,
		&bot.EditMessageReplyMarkupParams{ChatID: m.Message.Chat.ID, MessageID: m.Message.ID},
	)
	if err = ignoreNotModified(err); err != nil {
		h.log.WarnContext(ctx, "remove outdated setup buttons", "chat_id", m.Message.Chat.ID, "error", err)
	}
}
