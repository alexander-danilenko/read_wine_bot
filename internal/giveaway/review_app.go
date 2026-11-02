package giveaway

import (
	"context"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// Codes of results review failures, besides the App codes.
const (
	CodeReviewClosed    = "review_closed"    // the results were published or skipped already
	CodePublishedPartly = "published_partly" // some result messages reached the channel, not all
	CodeCannotMessage   = "cannot_message"   // the bot cannot send the caller a private message
)

// ReviewTelegramAll is what results review in the Mini App needs from Telegram. BotAPI
// implements it.
type ReviewTelegramAll interface {
	ReviewTelegram
	SubscriptionTelegram
}

// WinnerView is one winner as the review shows it.
type WinnerView struct {
	UserID    int64
	FirstName string
	LastName  string
	Username  string // without the @, "" for none
	Place     int
	// Subscriptions has one entry per channel of the ReviewView, in the same order.
	Subscriptions []Subscription
}

// ReviewView is the results review of a drawn giveaway.
type ReviewView struct {
	Giveaway View
	Winners  []WinnerView
	Channels []ChannelInfo // the channels whose subscriptions the review shows
	Rerolled int           // the rerolled-out count: publish and skip send it back (KTD11)
}

// reviewProblems maps the failure notices of results review to Mini App problems.
func reviewProblems() map[i18n.Key]AppError {
	return map[i18n.Key]AppError{
		i18n.ReviewNotAdmin:        {Code: CodeNotAdmin},
		i18n.ReviewPublishFailed:   {Code: CodeBotNoRights},
		i18n.ReviewPublishedPartly: {Code: CodePublishedPartly},
		i18n.ReviewSendFailed:      {Code: CodeCannotMessage},
		i18n.EditNotFound:          {Code: CodeNotFound},
		i18n.EditFailed:            {Code: CodeFailed},
	}
}

// reviewErr returns the problem a ReviewReply stands for, or nil when the action worked.
// staleCode is what a stale reply means for this action.
func reviewErr(r ReviewReply, staleCode string) error {
	switch {
	case r.Stale:
		return &AppError{Code: staleCode}
	case r.Notice == "", r.Notice == i18n.ReviewPublished, r.Notice == i18n.ReviewSkipped:
		return nil
	}

	if p, ok := reviewProblems()[r.Notice]; ok {
		return &p
	}

	return &AppError{Code: CodeFailed}
}

// Review returns the results review of giveaway id, to any current admin of its channel.
func (a *App) Review(ctx context.Context, tg ReviewTelegramAll, userID int64, id uint) (ReviewView, error) {
	r := a.reviewer.Show(ctx, tg, userID, id)
	if err := reviewErr(r, CodeReviewClosed); err != nil {
		return ReviewView{}, err
	}

	return a.reviewView(ctx, tg, r)
}

// Reroll replaces winner out of giveaway id with another eligible participant.
func (a *App) Reroll(ctx context.Context, tg ReviewTelegramAll, userID int64, id uint, out int64) (ReviewView, error) {
	r := a.reviewer.Reroll(ctx, tg, userID, id, out)
	if err := reviewErr(r, CodeStale); err != nil {
		return ReviewView{}, err
	}

	return a.reviewView(ctx, tg, r)
}

// Publish posts the results of giveaway id to its channel, if their rerolled-out count is
// still rerolled.
func (a *App) Publish(ctx context.Context, tg ReviewTelegramAll, userID int64, id uint, rerolled int) error {
	return reviewErr(a.reviewer.Publish(ctx, tg, userID, id, rerolled), CodeStale)
}

// SendWinners sends the current winners of giveaway id to the caller in a private message.
func (a *App) SendWinners(ctx context.Context, tg ReviewTelegramAll, userID int64, id uint) error {
	return reviewErr(a.reviewer.SendWinners(ctx, tg, userID, id), CodeStale)
}

// Skip closes the review of giveaway id without posting, if the rerolled-out count is still
// rerolled.
func (a *App) Skip(ctx context.Context, tg ReviewTelegramAll, userID int64, id uint, rerolled int) error {
	return reviewErr(a.reviewer.Skip(ctx, tg, userID, id, rerolled), CodeStale)
}

// reviewView returns the review r shows, with each winner's subscriptions.
func (a *App) reviewView(ctx context.Context, tg SubscriptionTelegram, r ReviewReply) (ReviewView, error) {
	g, err := a.view(ctx, r.Giveaway)
	if err != nil {
		return ReviewView{}, err
	}

	subs := a.reviewer.Subscriptions(ctx, tg, r.Giveaway, r.Results.Winners)
	winners := make([]WinnerView, 0, len(r.Results.Winners))

	for _, w := range r.Results.Winners {
		winners = append(winners, WinnerView{
			UserID: w.TelegramID, FirstName: w.FirstName, LastName: w.LastName, Username: w.Username,
			Place: w.Place, Subscriptions: subs.Of[w.TelegramID],
		})
	}

	return ReviewView{Giveaway: g, Winners: winners, Channels: subs.Channels, Rerolled: r.Results.Rerolled}, nil
}
