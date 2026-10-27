package giveaway

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/keyboard"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
)

// resultsPayload starts the start payload of a results link from the giveaway list.
const resultsPayload = "results_"

// ReviewLinkPrefix starts the message Telegram sends when the user taps a results link from
// the giveaway list: /start results_<channel>_<post>.
const ReviewLinkPrefix = "/start " + resultsPayload

// ReviewCallbackPrefix starts the callback data of every results review button. It is not a
// prefix of any other handler's callback data, so each handler gets only its own taps.
const ReviewCallbackPrefix = "giveaway-review:"

// Results review button actions: giveaway-review:<action>:<giveaway ID>[:<argument>]. The
// argument is a winner's Telegram ID for the reroll actions and the rerolled-out count the
// admin saw for the publish and skip actions.
const (
	reviewShow       = "show"   // shows the review
	reviewAskReroll  = "ask"    // asks to confirm replacing a winner
	reviewReroll     = "reroll" // replaces the winner
	reviewAskPublish = "askpub" // asks to confirm publishing
	reviewPublish    = "pub"    // publishes the results
	reviewAskSkip    = "asksk"  // asks to confirm closing without publishing
	reviewSkip       = "skip"   // closes without publishing
)

// Parts of a review button's callback data after the prefix: the action, the giveaway ID and
// the argument when there is one.
const (
	withoutArg = 2
	withArg    = 3
)

// rerollsPerRow is how many reroll buttons share a row of the review.
const rerollsPerRow = 5

// reviewData returns the callback data of a results review button.
func reviewData(action string, id uint, arg ...int64) string {
	parts := make([]string, 0, withoutArg+len(arg))
	parts = append(parts, action, strconv.FormatUint(uint64(id), 10))
	for _, a := range arg {
		parts = append(parts, strconv.FormatInt(a, 10))
	}

	return ReviewCallbackPrefix + strings.Join(parts, ":")
}

// reviewAction is a parsed results review button tap.
type reviewAction struct {
	name string
	id   uint
	arg  int64 // set for every action but show
}

// parseReviewData parses the callback data of a results review button and reports false for
// anything else, such as a button from an older version of the bot.
func parseReviewData(data string) (reviewAction, bool) {
	parts := strings.Split(strings.TrimPrefix(data, ReviewCallbackPrefix), ":")

	want := withArg
	switch parts[0] {
	case reviewShow:
		want = withoutArg
	case reviewAskReroll, reviewReroll, reviewAskPublish, reviewPublish, reviewAskSkip, reviewSkip:
	default:
		return reviewAction{}, false
	}

	if len(parts) != want {
		return reviewAction{}, false
	}

	id, err := strconv.ParseUint(parts[1], 10, 0)
	if err != nil {
		return reviewAction{}, false
	}

	a := reviewAction{name: parts[0], id: uint(id)}
	if want == withArg {
		if a.arg, err = strconv.ParseInt(parts[2], 10, 64); err != nil || a.arg < 0 {
			return reviewAction{}, false
		}
	}

	return a, true
}

// reviewView returns the text and buttons of the screen r shows. subs are shown on the
// results screen.
func reviewView(
	rv *Reviewer, lang string, r ReviewReply, subs Subscriptions,
) (string, [][]models.InlineKeyboardButton) {
	texts := rv.texts
	button := func(text, data string) models.InlineKeyboardButton {
		return models.InlineKeyboardButton{Text: text, CallbackData: data}
	}
	back := button(texts.Text(lang, i18n.EditNo), reviewData(reviewShow, r.Giveaway.ID))
	rerolled := int64(r.Results.Rerolled)

	switch r.Screen {
	case RerollScreen:
		i := slices.IndexFunc(r.Results.Winners, func(w Winner) bool { return w.TelegramID == r.Winner.TelegramID })

		return rv.RerollText(lang, i, r.Winner), [][]models.InlineKeyboardButton{
			{
				button(
					texts.Text(lang, i18n.ReviewYesReroll),
					reviewData(reviewReroll, r.Giveaway.ID, r.Winner.TelegramID),
				),
				back,
			},
		}
	case PublishScreen:
		return texts.Text(lang, i18n.ReviewAskPublish), [][]models.InlineKeyboardButton{{
			button(texts.Text(lang, i18n.ReviewYesPublish), reviewData(reviewPublish, r.Giveaway.ID, rerolled)),
			back,
		}}
	case SkipScreen:
		return texts.Text(lang, i18n.ReviewAskSkip), [][]models.InlineKeyboardButton{{
			button(texts.Text(lang, i18n.ReviewYesSkip), reviewData(reviewSkip, r.Giveaway.ID, rerolled)),
			back,
		}}
	case NoScreen, ResultsScreen:
	}

	rerolls := make([]models.InlineKeyboardButton, 0, len(r.Results.Winners))
	for i, w := range r.Results.Winners {
		rerolls = append(rerolls, button(
			texts.Text(lang, i18n.ReviewRerollButton, place(w, i)),
			reviewData(reviewAskReroll, r.Giveaway.ID, w.TelegramID),
		))
	}

	rows := slices.Collect(slices.Chunk(rerolls, rerollsPerRow))
	rows = append(rows,
		[]models.InlineKeyboardButton{
			button(texts.Text(lang, i18n.ReviewPublishButton), reviewData(reviewAskPublish, r.Giveaway.ID, rerolled)),
		},
		[]models.InlineKeyboardButton{
			button(texts.Text(lang, i18n.ReviewSkipButton), reviewData(reviewAskSkip, r.Giveaway.ID, rerolled)),
		},
	)

	return rv.ReviewText(lang, r.Giveaway, r.Results, subs), rows
}

// reviewHandler translates results review updates into Reviewer calls and their replies
// into messages.
type reviewHandler struct {
	rv    *Reviewer
	kb    *keyboard.Tracker
	texts *i18n.Catalog
	log   *slog.Logger
}

// reply sends the notice of r and shows its screen: in place of message tapped, or below the
// notice when tapped is 0.
func (h reviewHandler) reply(ctx context.Context, b *bot.Bot, userID int64, lang string, r ReviewReply, tapped int) {
	if r.Notice != "" {
		notify(ctx, b, h.log, userID, h.texts.Text(lang, r.Notice))
	}

	if r.Screen == NoScreen {
		return
	}

	var subs Subscriptions
	if r.Screen == ResultsScreen {
		subs = h.rv.Subscriptions(ctx, telegramAPI{b: b}, r.Giveaway, r.Results.Winners)
	}

	text, rows := reviewView(h.rv, lang, r, subs)
	showHTML(ctx, b, h.kb, tapped, userID, text, rows)
}

// ReviewCallbackHandler returns the handler for the results review buttons. The next screen
// replaces the tapped message; a tap that ends in a notice deletes it, and a button whose
// action no longer applies gets an "outdated" popup.
func ReviewCallbackHandler(
	rv *Reviewer, texts *i18n.Catalog, users language.Reader, kb *keyboard.Tracker, log *slog.Logger,
) bot.HandlerFunc {
	h := reviewHandler{rv: rv, kb: kb, texts: texts, log: log}

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		query := update.CallbackQuery
		if query == nil {
			return
		}

		userID := query.From.ID
		lang := language.UserLanguage(ctx, users, log, &query.From)

		r := ReviewReply{Stale: true}
		if a, ok := parseReviewData(query.Data); ok {
			r = h.act(ctx, telegramAPI{b: b}, userID, a)
		}

		popup := ""
		if r.Stale {
			popup = texts.Text(lang, i18n.GiveawayOutdated)
		}

		// Telegram shows a spinner on the button until the query is answered.
		_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: popup})
		if err != nil {
			log.ErrorContext(ctx, "answer results review button", "user_id", userID, "error", err)
		}

		if r.Stale {
			return
		}

		tapped := 0
		if r.Notice == "" && r.Screen != NoScreen && query.Message.Message != nil {
			tapped = query.Message.Message.ID
		}

		if tapped == 0 {
			kb.Discard(ctx, b, query.Message)
		}

		// Review buttons are sent only in private chats, where the chat ID equals the user ID.
		h.reply(ctx, b, userID, lang, r, tapped)
	}
}

// act runs the Reviewer call a tap asks for.
func (h reviewHandler) act(ctx context.Context, tg telegramAPI, userID int64, a reviewAction) ReviewReply {
	switch a.name {
	case reviewAskReroll:
		return h.rv.AskReroll(ctx, tg, userID, a.id, a.arg)
	case reviewReroll:
		return h.rv.Reroll(ctx, tg, userID, a.id, a.arg)
	case reviewAskPublish:
		return h.rv.AskPublish(ctx, tg, userID, a.id, int(a.arg))
	case reviewPublish:
		return h.rv.Publish(ctx, tg, userID, a.id, int(a.arg))
	case reviewAskSkip:
		return h.rv.AskSkip(ctx, tg, userID, a.id, int(a.arg))
	case reviewSkip:
		return h.rv.Skip(ctx, tg, userID, a.id, int(a.arg))
	default:
		return h.rv.Show(ctx, tg, userID, a.id)
	}
}

// ReviewLinkHandler returns the handler for the results links of the giveaway list: it shows
// the results review of the giveaway on the post the link names.
func ReviewLinkHandler(
	rv *Reviewer, texts *i18n.Catalog, users language.Reader, kb *keyboard.Tracker, log *slog.Logger,
) bot.HandlerFunc {
	h := reviewHandler{rv: rv, kb: kb, texts: texts, log: log}

	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		msg := privateMessage(update)
		if msg == nil {
			return
		}

		dropLinkTap(ctx, b, log, msg)

		lang := language.UserLanguage(ctx, users, log, msg.From)
		typing(ctx, b, log, msg.From.ID)

		chatID, messageID, ok := parsePostPayload(msg.Text, ReviewLinkPrefix)
		if !ok {
			h.reply(ctx, b, msg.From.ID, lang, ReviewReply{Notice: i18n.EditNotFound}, 0)

			return
		}

		h.reply(ctx, b, msg.From.ID, lang, rv.Open(ctx, telegramAPI{b: b}, msg.From.ID, chatID, messageID), 0)
	}
}

// ReviewNotifier sends a drawn giveaway's results to its creator for review.
type ReviewNotifier struct {
	h        reviewHandler
	creators Users
}

// NewReviewNotifier returns a ReviewNotifier.
func NewReviewNotifier(
	rv *Reviewer, creators Users, texts *i18n.Catalog, kb *keyboard.Tracker, log *slog.Logger,
) *ReviewNotifier {
	return &ReviewNotifier{h: reviewHandler{rv: rv, kb: kb, texts: texts, log: log}, creators: creators}
}

// SendReview sends the results review of g to its creator. While the creator is in a setup
// or edit step, a silent notice pointing to /list goes instead, so the step keeps its prompt
// and Cancel button. A failed send is logged: the results stay reachable through /list.
func (n *ReviewNotifier) SendReview(ctx context.Context, b *bot.Bot, g Giveaway) {
	log := n.h.log.With("giveaway_id", g.ID, "user_id", g.CreatedBy)

	lang := g.Language
	if saved, err := n.creators.Language(ctx, g.CreatedBy); err == nil && saved != "" {
		lang = saved
	}

	pending, err := n.creators.PendingCommand(ctx, g.CreatedBy)
	if err != nil {
		log.WarnContext(ctx, "read creator's pending step", "error", err)
	}

	if err != nil || pending != "" {
		send(ctx, b, log, &bot.SendMessageParams{
			ChatID:              g.CreatedBy,
			Text:                n.h.texts.Text(lang, i18n.ReviewWaiting, postLink(g.ChatID, g.MessageID)),
			ParseMode:           models.ParseModeHTML,
			LinkPreviewOptions:  &models.LinkPreviewOptions{IsDisabled: bot.True()},
			DisableNotification: true,
		})

		return
	}

	r, err := n.h.rv.Results(ctx, g)
	if err != nil {
		log.ErrorContext(ctx, "read giveaway results for review", "error", err)

		return
	}

	n.h.reply(ctx, b, g.CreatedBy, lang, r, 0)
}
