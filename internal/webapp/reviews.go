package webapp

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
)

// ReviewReader shows a results review. *giveaway.App implements it.
type ReviewReader interface {
	Review(ctx context.Context, tg giveaway.ReviewTelegramAll, userID int64, id uint) (giveaway.ReviewView, error)
}

// ReviewActions rerolls winners and publishes or skips results. *giveaway.App implements it.
type ReviewActions interface {
	Reroll(
		ctx context.Context,
		tg giveaway.ReviewTelegramAll,
		userID int64,
		id uint,
		out int64,
	) (giveaway.ReviewView, error)
	Publish(ctx context.Context, tg giveaway.ReviewTelegramAll, userID int64, id uint, rerolled int) error
	Skip(ctx context.Context, tg giveaway.ReviewTelegramAll, userID int64, id uint, rerolled int) error
	SendWinners(ctx context.Context, tg giveaway.ReviewTelegramAll, userID int64, id uint) error
}

type channelBody struct {
	ID       int64  `doc:"Telegram ID of the channel"          json:"id"`
	Title    string `doc:"Channel title"                       json:"title"`
	Username string `doc:"Public username without @, or empty" json:"username"`
}

type winnerBody struct {
	UserID        int64    `doc:"Telegram user ID"                                     json:"user_id"`
	FirstName     string   `doc:"First name"                                           json:"first_name"`
	LastName      string   `doc:"Last name, or empty"                                  json:"last_name"`
	Username      string   `doc:"Username without @, or empty"                         json:"username"`
	Place         int      `doc:"Place from 1"                                         json:"place"`
	Subscriptions []string `doc:"Subscription to each channel of the review, in order" json:"subscriptions"`
}

// TransformSchema lists the subscription states as an enum in the spec.
func (winnerBody) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	if subs, ok := s.Properties["subscriptions"]; ok && subs.Items != nil {
		subs.Items.Enum = []any{subscribed, notSubscribed, unknown}
	}

	return s
}

// Subscription states as the API writes them. unknown means the check failed, which is never
// shown as not subscribed.
const (
	subscribed    = "subscribed"
	notSubscribed = "not_subscribed"
	unknown       = "unknown"
)

func subscription(s giveaway.Subscription) string {
	switch s {
	case giveaway.Subscribed:
		return subscribed
	case giveaway.NotSubscribed:
		return notSubscribed
	case giveaway.SubscriptionUnknown:
		return unknown
	default:
		return unknown
	}
}

type reviewOutput struct {
	Body struct {
		Giveaway giveawayBody  `doc:"The drawn giveaway"                                  json:"giveaway"`
		Winners  []winnerBody  `doc:"Current winners in place order"                      json:"winners"`
		Channels []channelBody `doc:"Channels whose subscriptions the review shows"       json:"channels"`
		Rerolled int           `doc:"Rerolled-out count; send it back to publish or skip" json:"rerolled"`
	}
}

type rerollInput struct {
	ID   uint `doc:"Giveaway ID" path:"id"`
	Body struct {
		WinnerID int64 `doc:"Telegram user ID of the winner to replace" json:"winner_id"`
	}
}

type closeReviewInput struct {
	ID   uint `doc:"Giveaway ID" path:"id"`
	Body struct {
		Rerolled int `doc:"The rerolled count the review showed" json:"rerolled"`
	}
}

func (o *Operations) registerReviews(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getReview", Method: http.MethodGet, Path: "/api/reviews/{id}",
		Summary: "The results review of a drawn giveaway, for any current admin of its channel",
	}, o.getReview)
	huma.Register(api, huma.Operation{
		OperationID: "rerollWinner", Method: http.MethodPost, Path: "/api/reviews/{id}/reroll",
		Summary: "Replace one winner with another participant",
	}, o.reroll)
	huma.Register(api, huma.Operation{
		OperationID: "publishResults", Method: http.MethodPost, Path: "/api/reviews/{id}/publish",
		Summary: "Post the results as a reply to the giveaway post", DefaultStatus: http.StatusNoContent,
	}, o.publish)
	huma.Register(api, huma.Operation{
		OperationID: "skipResults", Method: http.MethodPost, Path: "/api/reviews/{id}/skip",
		Summary: "Close the review without posting the results", DefaultStatus: http.StatusNoContent,
	}, o.skip)
	huma.Register(api, huma.Operation{
		OperationID: "sendWinners", Method: http.MethodPost, Path: "/api/reviews/{id}/send-winners",
		Summary:       "Send the caller the current winners in a private message, to forward to partners",
		DefaultStatus: http.StatusNoContent,
	}, o.sendWinners)
}

func (o *Operations) getReview(ctx context.Context, in *idInput) (*reviewOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	v, err := o.reviewReader.Review(ctx, o.tg, c.User.ID, in.ID)

	return o.review(ctx, v, err)
}

func (o *Operations) reroll(ctx context.Context, in *rerollInput) (*reviewOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	v, err := o.reviewActions.Reroll(ctx, o.tg, c.User.ID, in.ID, in.Body.WinnerID)

	return o.review(ctx, v, err)
}

func (o *Operations) publish(ctx context.Context, in *closeReviewInput) (*struct{}, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	if err := o.reviewActions.Publish(ctx, o.tg, c.User.ID, in.ID, in.Body.Rerolled); err != nil {
		return nil, o.apiErr(ctx, err)
	}

	return &struct{}{}, nil
}

func (o *Operations) skip(ctx context.Context, in *closeReviewInput) (*struct{}, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	if err := o.reviewActions.Skip(ctx, o.tg, c.User.ID, in.ID, in.Body.Rerolled); err != nil {
		return nil, o.apiErr(ctx, err)
	}

	return &struct{}{}, nil
}

func (o *Operations) sendWinners(ctx context.Context, in *idInput) (*struct{}, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	if err := o.reviewActions.SendWinners(ctx, o.tg, c.User.ID, in.ID); err != nil {
		return nil, o.apiErr(ctx, err)
	}

	return &struct{}{}, nil
}

// review returns v, or the API error for err.
func (o *Operations) review(ctx context.Context, v giveaway.ReviewView, err error) (*reviewOutput, error) {
	if err != nil {
		return nil, o.apiErr(ctx, err)
	}

	out := &reviewOutput{}
	out.Body.Giveaway, out.Body.Rerolled = toBody(v.Giveaway), v.Rerolled

	out.Body.Channels = make([]channelBody, 0, len(v.Channels))
	for _, ch := range v.Channels {
		out.Body.Channels = append(out.Body.Channels, channelBody{ID: ch.ID, Title: ch.Title, Username: ch.Username})
	}

	out.Body.Winners = make([]winnerBody, 0, len(v.Winners))
	for _, w := range v.Winners {
		subs := make([]string, 0, len(w.Subscriptions))
		for _, s := range w.Subscriptions {
			subs = append(subs, subscription(s))
		}

		out.Body.Winners = append(out.Body.Winners, winnerBody{
			UserID: w.UserID, FirstName: w.FirstName, LastName: w.LastName, Username: w.Username, Place: w.Place,
			Subscriptions: subs,
		})
	}

	return out, nil
}
