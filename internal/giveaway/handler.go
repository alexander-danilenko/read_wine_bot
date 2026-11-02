package giveaway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/channel"
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

// BotAPI implements Telegram with the bot that received the update.
type BotAPI struct {
	b *bot.Bot
}

// NewBotAPI returns the Telegram calls the giveaway rules make, through b.
func NewBotAPI(b *bot.Bot) BotAPI {
	return BotAPI{b: b}
}

// ErrNotChannel means a post link names a chat that is not a channel.
var ErrNotChannel = errors.New("chat is not a channel")

// ResolveChannel returns the chat ID of chat, an @username or a chat ID as parsePostLink
// returns it, after checking that it is a channel.
func (t BotAPI) ResolveChannel(ctx context.Context, chat string) (int64, error) {
	info, err := t.b.GetChat(ctx, &bot.GetChatParams{ChatID: chat})
	if err != nil {
		return 0, err //nolint:wrapcheck // App logs it with the link
	}

	if info.Type != models.ChatTypeChannel {
		return 0, ErrNotChannel
	}

	return info.ID, nil
}

func (t BotAPI) IsChannelAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	m, err := t.b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: chatID, UserID: userID})
	if err != nil {
		return false, err //nolint:wrapcheck // Service logs it with the chat and user IDs
	}

	return m.Type == models.ChatMemberTypeOwner || m.Type == models.ChatMemberTypeAdministrator, nil
}

func (t BotAPI) Channel(ctx context.Context, chatID int64) (ChannelInfo, error) {
	c, err := t.b.GetChat(ctx, &bot.GetChatParams{ChatID: chatID})
	if err != nil {
		return ChannelInfo{}, err //nolint:wrapcheck // Reviewer logs it with the chat ID
	}

	info := ChannelInfo{ID: c.ID, Title: c.Title, Username: c.Username}
	if c.Photo != nil {
		info.PhotoID = c.Photo.SmallFileID
	}

	return info, nil
}

// maxPhotoSize caps a downloaded channel photo; the small one is a 160x160 JPEG of a few KB.
const (
	maxPhotoSize    = 64 << 10
	photoGetTimeout = 5 * time.Second
)

// errPhotoDownload stands for every download failure: the HTTP client's own errors quote the
// download URL, which carries the bot token, so they never reach a log.
var errPhotoDownload = errors.New("download channel photo")

// ChannelPhoto downloads photo fileID and returns it with its content type, so the Mini App
// gets the photo without ever seeing the download link.
func (t BotAPI) ChannelPhoto(ctx context.Context, fileID string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, photoGetTimeout)
	defer cancel()

	f, err := t.b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, "", fmt.Errorf("get channel photo file: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.b.FileDownloadLink(f), nil)
	if err != nil {
		return nil, "", errPhotoDownload
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", errPhotoDownload
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPhotoSize+1))
	if err != nil || resp.StatusCode != http.StatusOK || len(data) > maxPhotoSize {
		return nil, "", errPhotoDownload
	}

	kind := http.DetectContentType(data)
	if !strings.HasPrefix(kind, "image/") {
		return nil, "", fmt.Errorf("%w: got %s", errPhotoDownload, kind)
	}

	return data, kind, nil
}

func (t BotAPI) IsSubscribed(ctx context.Context, chatID, userID int64) (bool, error) {
	m, err := t.b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: chatID, UserID: userID})
	if err != nil {
		return false, err //nolint:wrapcheck // Reviewer logs it with the chat and user IDs
	}

	return channel.Subscribed(*m), nil
}

func (t BotAPI) CanRunGiveaways(ctx context.Context, chatID int64) (bool, error) {
	m, err := t.b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: chatID, UserID: t.b.ID()})
	if err != nil {
		return false, err //nolint:wrapcheck // Service logs it with the chat ID
	}

	return canRunGiveaways(*m), nil
}

// requiredRights returns the only admin rights a giveaway needs in its channel: Edit messages of
// others for the join button and Post messages for the results.
func requiredRights() models.ChatAdministratorRights {
	return models.ChatAdministratorRights{CanEditMessages: true, CanPostMessages: true}
}

// RequestRights makes Telegram pre-select requiredRights, and no other right, when an admin
// adds the bot to a channel. The admin can still change them.
func RequestRights(ctx context.Context, b *bot.Bot) error {
	rights := requiredRights()

	_, err := b.SetMyDefaultAdministratorRights(ctx, &bot.SetMyDefaultAdministratorRightsParams{
		Rights:      &rights,
		ForChannels: true,
	})
	if err != nil {
		return fmt.Errorf("set default channel admin rights: %w", err)
	}

	return nil
}

// AddToChannelLink returns the link that opens Telegram's channel picker for the bot named
// botUsername, with requiredRights already enabled.
func AddToChannelLink(botUsername string) string {
	return "https://t.me/" + botUsername + "?startchannel&admin=post_messages+edit_messages"
}

// AddToPartnerChannelLink returns the link that opens Telegram's channel picker for the bot
// named botUsername with no right the bot uses. A channel link must ask for at least one right;
// manage_chat is the one every admin has anyway.
func AddToPartnerChannelLink(botUsername string) string {
	return "https://t.me/" + botUsername + "?startchannel&admin=manage_chat"
}

// canRunGiveaways reports whether the bot's status in a channel has both rights in
// requiredRights.
func canRunGiveaways(m models.ChatMember) bool {
	return m.Administrator != nil && m.Administrator.CanEditMessages && m.Administrator.CanPostMessages
}

func (t BotAPI) AttachButton(ctx context.Context, chatID, messageID int64, label string) error {
	_, err := t.b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:    chatID,
		MessageID: int(messageID),
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: label, CallbackData: JoinCallback},
		}}},
	})

	return markGone(ignoreNotModified(err))
}

// RemoveButton takes every inline button off the channel post. A post without buttons, or a
// deleted post, counts as success.
func (t BotAPI) RemoveButton(ctx context.Context, chatID, messageID int64) error {
	_, err := t.b.EditMessageReplyMarkup(
		ctx,
		&bot.EditMessageReplyMarkupParams{ChatID: chatID, MessageID: int(messageID)},
	)

	return ignoreGone(ignoreNotModified(err))
}

// SendHTML sends an HTML text without a link preview, as a reply to message replyTo of the
// same chat unless replyTo is 0. When replyTo was deleted, the text goes without the reply.
func (t BotAPI) SendHTML(ctx context.Context, chatID, replyTo int64, text string) error {
	params := &bot.SendMessageParams{
		ChatID:             chatID,
		Text:               text,
		ParseMode:          models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
	}
	if replyTo != 0 {
		params.ReplyParameters = &models.ReplyParameters{MessageID: int(replyTo), AllowSendingWithoutReply: true}
	}

	_, err := t.b.SendMessage(ctx, params)

	return err //nolint:wrapcheck // Reviewer logs it with the giveaway, chat and message IDs
}

// ignoreNotModified treats Telegram's "message is not modified" answer as success: it means
// the post already carries this exact button, for example after a retry.
func ignoreNotModified(err error) error {
	if errors.Is(err, bot.ErrorBadRequest) && strings.Contains(err.Error(), "message is not modified") {
		return nil
	}

	return err
}

// ignoreGone treats Telegram's answer for a deleted post as success: a deleted post
// carries no button.
func ignoreGone(err error) error {
	if isGone(err) {
		return nil
	}

	return err
}

// markGone wraps Telegram's answer for a deleted post as ErrPostGone.
func markGone(err error) error {
	if isGone(err) {
		return fmt.Errorf("%w: %w", ErrPostGone, err)
	}

	return err
}

// isGone reports whether err says the post to edit no longer exists. Telegram answers
// "message to edit not found" in private chats and "MESSAGE_ID_INVALID" for channel posts.
func isGone(err error) bool {
	if !errors.Is(err, bot.ErrorBadRequest) {
		return false
	}

	text := err.Error()

	return strings.Contains(text, "message to edit not found") || strings.Contains(text, "MESSAGE_ID_INVALID")
}
