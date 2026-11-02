package giveaway

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// CreatorLanguages reads the language a creator chose in the bot. *user.Store implements it.
type CreatorLanguages interface {
	Language(ctx context.Context, telegramID int64) (string, error)
}

// Sender sends the one message with buttons in a user's private chat. *keyboard.Tracker
// implements it.
type Sender interface {
	SendHTML(ctx context.Context, b *bot.Bot, userID int64, text string, rows [][]models.InlineKeyboardButton)
}

// ReviewNotifier tells a drawn giveaway's creator that its results wait for review, with a
// button that opens the review in the Mini App.
type ReviewNotifier struct {
	creators CreatorLanguages
	texts    *i18n.Catalog
	kb       Sender
	appURL   string
	log      *slog.Logger
}

// NewReviewNotifier returns a ReviewNotifier whose button opens the Mini App at appURL.
func NewReviewNotifier(
	creators CreatorLanguages, texts *i18n.Catalog, kb Sender, appURL string, log *slog.Logger,
) *ReviewNotifier {
	return &ReviewNotifier{creators: creators, texts: texts, kb: kb, appURL: appURL, log: log}
}

// ReviewAppURL returns the Mini App link that opens the review of giveaway id. The query only
// chooses the screen; the API checks who may see it.
func ReviewAppURL(appURL string, id uint) string {
	return appURL + "?review=" + strconv.FormatUint(uint64(id), 10)
}

// SendReview tells the creator of g, in the language they chose or else g's, that its results
// wait for review. Any current admin of the channel can review them in the app; the creator is
// the one the bot tells.
func (n *ReviewNotifier) SendReview(ctx context.Context, b *bot.Bot, g Giveaway) {
	lang := g.Language
	if saved, err := n.creators.Language(ctx, g.CreatedBy); err != nil {
		n.log.WarnContext(ctx, "read creator's language", "user_id", g.CreatedBy, "error", err)
	} else if saved != "" {
		lang = saved
	}

	open := models.InlineKeyboardButton{
		Text:   n.texts.Text(lang, i18n.ReviewOpenApp),
		WebApp: &models.WebAppInfo{URL: ReviewAppURL(n.appURL, g.ID)},
	}
	n.kb.SendHTML(ctx, b, g.CreatedBy, n.texts.Text(lang, i18n.ReviewReady, postLink(g.ChatID, g.MessageID)),
		[][]models.InlineKeyboardButton{{open}})
}
