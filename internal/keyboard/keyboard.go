// Package keyboard keeps at most one message with inline buttons in each user's private chat:
// a new one replaces the previous, so the chat never fills with buttons that no longer work.
package keyboard

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Store saves the message with buttons in each user's private chat. *user.Store implements it.
type Store interface {
	KeyboardMessage(ctx context.Context, telegramID int64) (int64, error)
	SetKeyboardMessage(ctx context.Context, telegramID, messageID int64) error
}

// Tracker sends, replaces and deletes the message with buttons in each user's private chat.
// Private chat IDs equal user IDs.
type Tracker struct {
	store Store
	log   *slog.Logger
}

// NewTracker returns a Tracker that remembers each user's message with buttons in store.
func NewTracker(store Store, log *slog.Logger) *Tracker {
	return &Tracker{store: store, log: log}
}

// Send deletes the user's previous message with buttons, then sends text with rows.
func (t *Tracker) Send(
	ctx context.Context,
	b *bot.Bot,
	userID int64,
	text string,
	rows [][]models.InlineKeyboardButton,
) {
	t.drop(ctx, b, userID, 0)

	m, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      userID,
		Text:        text,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	if err != nil {
		t.log.ErrorContext(ctx, "send message with buttons", "user_id", userID, "error", err)

		return
	}

	t.save(ctx, userID, int64(m.ID))
}

// Replace shows text with rows in place of messageID, the message whose button the user
// tapped, so a step within one screen does not move it down the chat. When the edit fails,
// it deletes messageID and sends a new message instead.
func (t *Tracker) Replace(
	ctx context.Context,
	b *bot.Bot,
	userID int64,
	messageID int,
	text string,
	rows [][]models.InlineKeyboardButton,
) {
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      userID,
		MessageID:   messageID,
		Text:        text,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	if err != nil && !isNotModified(err) {
		t.log.WarnContext(ctx, "replace message with buttons", "user_id", userID, "error", err)
		discard(ctx, b, t.log, userID, messageID)
		t.Send(ctx, b, userID, text, rows)

		return
	}

	t.drop(ctx, b, userID, messageID)
	t.save(ctx, userID, int64(messageID))
}

// Clear deletes the user's message with buttons, if there is one.
func (t *Tracker) Clear(ctx context.Context, b *bot.Bot, userID int64) {
	if t.drop(ctx, b, userID, 0) {
		t.save(ctx, userID, 0)
	}
}

// Discard deletes the message a tapped button belongs to.
func (t *Tracker) Discard(ctx context.Context, b *bot.Bot, m models.MaybeInaccessibleMessage) {
	if m.Message != nil {
		discard(ctx, b, t.log, m.Message.Chat.ID, m.Message.ID)
	}
}

// drop deletes the user's saved message with buttons unless it is keep, and reports whether
// there was one.
func (t *Tracker) drop(ctx context.Context, b *bot.Bot, userID int64, keep int) bool {
	messageID, err := t.store.KeyboardMessage(ctx, userID)
	if err != nil {
		t.log.ErrorContext(ctx, "read keyboard message", "user_id", userID, "error", err)

		return false
	}

	if messageID == 0 {
		return false
	}

	if messageID != int64(keep) {
		discard(ctx, b, t.log, userID, int(messageID))
	}

	return true
}

// save remembers messageID as the user's message with buttons; 0 forgets it.
func (t *Tracker) save(ctx context.Context, userID, messageID int64) {
	if err := t.store.SetKeyboardMessage(ctx, userID, messageID); err != nil {
		t.log.ErrorContext(ctx, "save keyboard message", "user_id", userID, "error", err)
	}
}

// discard deletes a bot message. When Telegram refuses, as it does 48 hours after sending, it
// removes the message's buttons instead. A message already deleted counts as success.
func discard(ctx context.Context, b *bot.Bot, log *slog.Logger, chatID int64, messageID int) {
	_, err := b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: messageID})
	if err == nil || isGone(err) {
		return
	}

	_, err = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{ChatID: chatID, MessageID: messageID})
	if err != nil && !isNotModified(err) && !isGone(err) {
		log.WarnContext(ctx, "discard message", "chat_id", chatID, "message_id", messageID, "error", err)
	}
}

// isGone reports whether err says the message no longer exists.
func isGone(err error) bool {
	return errors.Is(err, bot.ErrorBadRequest) && strings.Contains(err.Error(), "not found")
}

// isNotModified reports whether err says the message already looks as asked.
func isNotModified(err error) bool {
	return errors.Is(err, bot.ErrorBadRequest) && strings.Contains(err.Error(), "message is not modified")
}
