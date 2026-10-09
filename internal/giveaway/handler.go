package giveaway

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
)

// JoinCallback is the callback data of every join button. The post itself identifies the
// giveaway, so the data needs nothing else.
const JoinCallback = "giveaway:join"

// JoinHandler returns a handler for taps on the join button. It records the participant and
// answers with a popup in the giveaway's language, or the tapper's when no giveaway runs on
// the post.
func JoinHandler(svc *Service, texts *i18n.Catalog, users language.Reader, log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		query := update.CallbackQuery
		if query == nil {
			return
		}

		result, lang := JoinFailed, ""
		if chatID, messageID, ok := post(query.Message); ok {
			result, lang = svc.Join(ctx, chatID, messageID, query.From.ID)
		}

		if lang == "" {
			lang = language.UserLanguage(ctx, users, log, &query.From)
		}

		_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            texts.Text(lang, joinText(result)),
		})
		if err != nil {
			log.ErrorContext(ctx, "answer join button", "user_id", query.From.ID, "error", err)
		}
	}
}

// post returns the chat ID and the message ID of the post a button belongs to, and false if
// the tap carries neither. Telegram sends old posts as inaccessible messages, which still
// carry both IDs.
func post(m models.MaybeInaccessibleMessage) (int64, int64, bool) {
	switch {
	case m.Message != nil:
		return m.Message.Chat.ID, int64(m.Message.ID), true
	case m.InaccessibleMessage != nil:
		return m.InaccessibleMessage.Chat.ID, int64(m.InaccessibleMessage.MessageID), true
	default:
		return 0, 0, false
	}
}

func startText(r StartResult) i18n.Key {
	switch r {
	case Created:
		return i18n.GiveawayCreated
	case Exists:
		return i18n.GiveawayExists
	case Taken:
		return i18n.GiveawayTaken
	case NotAdmin:
		return i18n.GiveawayNotAdmin
	case NoRights:
		return i18n.GiveawayNoRights
	case Failed:
		return i18n.GiveawayFailed
	default:
		return i18n.GiveawayFailed
	}
}

func joinText(r JoinResult) i18n.Key {
	switch r {
	case Joined:
		return i18n.GiveawayJoined
	case AlreadyJoined:
		return i18n.GiveawayAlreadyJoined
	case JoinFailed:
		return i18n.GiveawayJoinFailed
	case Ended:
		return i18n.GiveawayEnded
	default:
		return i18n.GiveawayJoinFailed
	}
}

// telegramAPI implements Telegram with the bot that received the update.
type telegramAPI struct {
	b *bot.Bot
}

func (t telegramAPI) IsChannelAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	m, err := t.b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: chatID, UserID: userID})
	if err != nil {
		return false, err //nolint:wrapcheck // Service logs it with the chat and user IDs
	}

	return m.Type == models.ChatMemberTypeOwner || m.Type == models.ChatMemberTypeAdministrator, nil
}

func (t telegramAPI) CanEditPosts(ctx context.Context, chatID int64) (bool, error) {
	m, err := t.b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: chatID, UserID: t.b.ID()})
	if err != nil {
		return false, err //nolint:wrapcheck // Service logs it with the chat ID
	}

	return m.Administrator != nil && m.Administrator.CanEditMessages, nil
}

func (t telegramAPI) AttachButton(ctx context.Context, chatID, messageID int64, label string) error {
	_, err := t.b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:    chatID,
		MessageID: int(messageID),
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: label, CallbackData: JoinCallback},
		}}},
	})

	return ignoreNotModified(err)
}

// RemoveButton takes every inline button off the channel post. A post without buttons
// counts as success.
func (t telegramAPI) RemoveButton(ctx context.Context, chatID, messageID int64) error {
	_, err := t.b.EditMessageReplyMarkup(
		ctx,
		&bot.EditMessageReplyMarkupParams{ChatID: chatID, MessageID: int(messageID)},
	)

	return ignoreNotModified(err)
}

// SendHTML sends an HTML text without a link preview, as a reply to message replyTo of the
// same chat unless replyTo is 0.
func (t telegramAPI) SendHTML(ctx context.Context, chatID, replyTo int64, text string) error {
	params := &bot.SendMessageParams{
		ChatID:             chatID,
		Text:               text,
		ParseMode:          models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
	}
	if replyTo != 0 {
		params.ReplyParameters = &models.ReplyParameters{MessageID: int(replyTo)}
	}

	_, err := t.b.SendMessage(ctx, params)

	return err //nolint:wrapcheck // Drawer logs it with the giveaway, chat and message IDs
}

// ignoreNotModified treats Telegram's "message is not modified" answer as success: it means
// the post already carries this exact button, for example after a retry.
func ignoreNotModified(err error) error {
	if errors.Is(err, bot.ErrorBadRequest) && strings.Contains(err.Error(), "message is not modified") {
		return nil
	}

	return err
}
