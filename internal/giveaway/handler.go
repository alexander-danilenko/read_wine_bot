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

// Command is the pending command that makes the bot wait for a forwarded post.
const Command = "giveaway"

// JoinCallback is the callback data of every join button. The post itself identifies the
// giveaway, so the data needs nothing else.
const JoinCallback = "giveaway:join"

// Users reads a user's saved language and takes their pending command. *user.Store implements it.
type Users interface {
	language.Reader
	MovePendingCommand(ctx context.Context, telegramID int64, from, to string) (bool, error)
	SetPendingCommand(ctx context.Context, telegramID int64, command string) error
}

// CommandHandler returns a handler that answers the giveaway menu button in a private chat:
// it marks the user as waiting for a forwarded post and asks for it.
func CommandHandler(texts *i18n.Catalog, users Users, log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
			return
		}

		if err := users.SetPendingCommand(ctx, msg.From.ID, Command); err != nil {
			log.ErrorContext(ctx, "start giveaway setup", "user_id", msg.From.ID, "error", err)

			return
		}

		lang := language.UserLanguage(ctx, users, log, msg.From)

		_, err := b.SendMessage(
			ctx,
			&bot.SendMessageParams{ChatID: msg.Chat.ID, Text: texts.Text(lang, i18n.GiveawayPrompt)},
		)
		if err != nil {
			log.ErrorContext(ctx, "send giveaway prompt", "chat_id", msg.Chat.ID, "error", err)
		}
	}
}

// ForwardHandler returns the handler for messages no other handler matched. It acts only on a
// channel post forwarded in a private chat by a user who tapped the giveaway button; everything
// else is ignored silently. The forward ends the wait whatever the outcome.
func ForwardHandler(svc *Service, texts *i18n.Catalog, users Users, log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate ||
			msg.ForwardOrigin == nil || msg.ForwardOrigin.MessageOriginChannel == nil {
			return
		}

		// Taking the pending command ends the wait whatever Start returns, and only one of
		// several posts forwarded together can take it.
		took, err := users.MovePendingCommand(ctx, msg.From.ID, Command, "")
		if err != nil {
			log.ErrorContext(ctx, "take pending command", "user_id", msg.From.ID, "error", err)

			return
		}

		if !took {
			return
		}

		origin := msg.ForwardOrigin.MessageOriginChannel
		lang := language.UserLanguage(ctx, users, log, msg.From)
		result := svc.Start(ctx, telegramAPI{b: b}, msg.From.ID, origin.Chat.ID, int64(origin.MessageID), lang)

		_, err = b.SendMessage(
			ctx,
			&bot.SendMessageParams{ChatID: msg.Chat.ID, Text: texts.Text(lang, startText(result))},
		)
		if err != nil {
			log.ErrorContext(ctx, "send giveaway result", "chat_id", msg.Chat.ID, "error", err)
		}
	}
}

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

// ignoreNotModified treats Telegram's "message is not modified" answer as success: it means
// the post already carries this exact button, for example after a retry.
func ignoreNotModified(err error) error {
	if errors.Is(err, bot.ErrorBadRequest) && strings.Contains(err.Error(), "message is not modified") {
		return nil
	}

	return err
}
