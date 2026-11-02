package webapp

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
)

// GiveawayReader lists and reads the caller's giveaways. *giveaway.App implements it.
type GiveawayReader interface {
	List(
		ctx context.Context,
		tg giveaway.Telegram,
		userID int64,
		offset, limit int,
	) (giveaway.Page, []giveaway.View, error)
	Channels(ctx context.Context, tg giveaway.ChannelReader, lists ...[]giveaway.View) []giveaway.Channel
	Archived(ctx context.Context, userID int64, offset, limit int) (giveaway.Page, error)
	Get(ctx context.Context, userID int64, id uint) (giveaway.View, error)
}

// GiveawayWriter creates and edits giveaways. *giveaway.App implements it.
type GiveawayWriter interface {
	Create(
		ctx context.Context,
		tg giveaway.AppTelegram,
		userID int64,
		lang string,
		f giveaway.Fields,
	) (giveaway.View, error)
	Edit(ctx context.Context, tg giveaway.Telegram, userID int64, id uint, c giveaway.Changes) (giveaway.View, error)
	Move(ctx context.Context, tg giveaway.AppTelegram, userID int64, id uint, link string) (giveaway.View, error)
}

// GiveawayActions cancels, restores and finishes giveaways. *giveaway.App implements it.
type GiveawayActions interface {
	Cancel(ctx context.Context, tg giveaway.AppTelegram, userID int64, id uint) (giveaway.View, error)
	Restore(ctx context.Context, tg giveaway.Telegram, userID int64, id uint) (giveaway.View, error)
	FinishNow(ctx context.Context, tg giveaway.Telegram, userID int64, id uint) (giveaway.View, error)
}

// giveawayBody is a giveaway as the API returns it.
type giveawayBody struct {
	ID           uint   `doc:"Giveaway ID"                     json:"id"`
	PostLink     string `doc:"Link to the channel post"        json:"post_link"`
	ChatID       int64  `doc:"Telegram ID of the channel"      json:"chat_id"`
	MessageID    int64  `doc:"Telegram ID of the post"         json:"message_id"`
	EndsAt       string `doc:"Kyiv end time, YYYY-MM-DDTHH:MM" json:"ends_at"`
	Winners      int    `doc:"Number of winners to draw"       json:"winners"`
	ButtonText   string `doc:"Label of the join button"        json:"button_text"`
	Status       string `doc:"Where the giveaway is"           json:"status"`
	Participants int    `doc:"Number of users who joined"      json:"participants"`
}

// TransformSchema lists the statuses as an enum in the spec, so the generated client knows them.
func (giveawayBody) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	if status, ok := s.Properties["status"]; ok {
		status.Enum = []any{
			giveaway.StatusActive, giveaway.StatusDue, giveaway.StatusCancelled, giveaway.StatusReview,
			giveaway.StatusFinished,
		}
	}

	return s
}

// listChannelBody is a channel as the lists and the giveaway screen show it, read live from Telegram.
type listChannelBody struct {
	ID       int64  `doc:"Telegram ID of the channel"                             json:"id"`
	Title    string `doc:"Channel title, empty if Telegram cannot give it"        json:"title"`
	Username string `doc:"Public username without @, empty for a private channel" json:"username"`
	Photo    string `doc:"URL of the small channel photo, empty for none"         json:"photo"`
}

func (o *Operations) toListChannels(cs []giveaway.Channel) []listChannelBody {
	out := make([]listChannelBody, 0, len(cs))
	for _, c := range cs {
		body := listChannelBody{ID: c.ID, Title: c.Title, Username: c.Username}
		if c.PhotoID != "" {
			body.Photo = o.photos.URL(c.PhotoID)
		}

		out = append(out, body)
	}

	return out
}

func toBody(v giveaway.View) giveawayBody {
	return giveawayBody{
		ID: v.ID, PostLink: v.PostLink, ChatID: v.ChatID, MessageID: v.MessageID, EndsAt: v.EndsAt,
		Winners: v.Winners, ButtonText: v.ButtonText, Status: string(v.Status), Participants: v.Participants,
	}
}

func toBodies(vs []giveaway.View) []giveawayBody {
	bodies := make([]giveawayBody, 0, len(vs))
	for _, v := range vs {
		bodies = append(bodies, toBody(v))
	}

	return bodies
}

type giveawayOutput struct {
	Body giveawayBody
}

type giveawayDetailsOutput struct {
	Body struct {
		Giveaway giveawayBody    `doc:"The giveaway"                         json:"giveaway"`
		Channel  listChannelBody `doc:"Its channel, read live from Telegram" json:"channel"`
	}
}

type idInput struct {
	ID uint `doc:"Giveaway ID" path:"id"`
}

// pageInput picks one page of a giveaway list.
type pageInput struct {
	Offset int `default:"0"  doc:"How many giveaways to skip" maximum:"100000" minimum:"0" query:"offset"`
	Limit  int `default:"10" doc:"Page size"                  maximum:"100"    minimum:"1" query:"limit"`
}

type listOutput struct {
	Body struct {
		Giveaways []giveawayBody    `doc:"One page of the caller's running giveaways, newest first"                      json:"giveaways"`
		Total     int               `doc:"How many running giveaways the caller has"                                     json:"total"`
		Review    []giveawayBody    `doc:"Giveaways whose results wait for review in channels the caller is an admin of" json:"review"`
		Channels  []listChannelBody `doc:"The channels of these giveaways, each once"                                    json:"channels"`
	}
}

type archiveOutput struct {
	Body struct {
		Giveaways []giveawayBody    `doc:"One page of the caller's finished and cancelled giveaways, the latest to end first" json:"giveaways"`
		Total     int               `doc:"How many finished and cancelled giveaways the caller has"                           json:"total"`
		Channels  []listChannelBody `doc:"The channels of these giveaways, each once"                                         json:"channels"`
	}
}

type createInput struct {
	Body struct {
		PostLink   string `doc:"t.me link to the channel post"                   json:"post_link"`
		EndsAt     string `doc:"End time in Kyiv, YYYY-MM-DDTHH:MM"              json:"ends_at"`
		Winners    int    `doc:"Number of winners to draw, 1 to 98"              json:"winners"`
		ButtonText string `doc:"Label of the join button, at most 64 characters" json:"button_text"`
	}
}

type editInput struct {
	ID   uint `doc:"Giveaway ID" path:"id"`
	Body struct {
		EndsAt     *string `doc:"New end time in Kyiv, YYYY-MM-DDTHH:MM" json:"ends_at,omitempty"     required:"false"`
		Winners    *int    `doc:"New number of winners, 1 to 98"         json:"winners,omitempty"     required:"false"`
		ButtonText *string `doc:"New label of the join button"           json:"button_text,omitempty" required:"false"`
	}
}

type moveInput struct {
	ID   uint `doc:"Giveaway ID" path:"id"`
	Body struct {
		PostLink string `doc:"t.me link to another post of the same channel" json:"post_link"`
	}
}

func (o *Operations) registerGiveaways(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listGiveaways", Method: http.MethodGet, Path: "/api/giveaways",
		Summary: "The caller's running giveaways page by page, and results waiting for review",
	}, o.listGiveaways)
	huma.Register(api, huma.Operation{
		OperationID: "listFinishedGiveaways", Method: http.MethodGet, Path: "/api/giveaways/finished",
		Summary: "The caller's finished and cancelled giveaways, page by page",
	}, o.listFinished)
	huma.Register(api, huma.Operation{
		OperationID: "createGiveaway", Method: http.MethodPost, Path: "/api/giveaways",
		Summary: "Create a giveaway and put its join button on the post", DefaultStatus: http.StatusCreated,
	}, o.createGiveaway)
	huma.Register(api, huma.Operation{
		OperationID: "getGiveaway", Method: http.MethodGet, Path: "/api/giveaways/{id}",
		Summary: "One of the caller's giveaways",
	}, o.getGiveaway)
	huma.Register(api, huma.Operation{
		OperationID: "editGiveaway", Method: http.MethodPatch, Path: "/api/giveaways/{id}",
		Summary: "Change the end time, winner count or button text",
	}, o.editGiveaway)
	huma.Register(api, huma.Operation{
		OperationID: "moveGiveaway", Method: http.MethodPost, Path: "/api/giveaways/{id}/move",
		Summary: "Move the giveaway to another post of its channel",
	}, o.moveGiveaway)
	o.registerActions(api)
}

func (o *Operations) registerActions(api huma.API) {
	actions := []struct {
		id, path, summary string
		act               func(ctx context.Context, userID int64, id uint) (giveaway.View, error)
	}{
		{
			"cancelGiveaway",
			"cancel",
			"Take the giveaway off its post",
			func(ctx context.Context, userID int64, id uint) (giveaway.View, error) {
				return o.actions.Cancel(ctx, o.tg, userID, id)
			},
		},
		{
			"restoreGiveaway",
			"restore",
			"Put a cancelled giveaway back on its post",
			func(ctx context.Context, userID int64, id uint) (giveaway.View, error) {
				return o.actions.Restore(ctx, o.tg, userID, id)
			},
		},
		{
			"finishGiveawayNow",
			"finish-now",
			"End the giveaway now; the draw follows within a minute",
			func(ctx context.Context, userID int64, id uint) (giveaway.View, error) {
				return o.actions.FinishNow(ctx, o.tg, userID, id)
			},
		},
	}

	for _, a := range actions {
		huma.Register(api, huma.Operation{
			OperationID: a.id, Method: http.MethodPost, Path: "/api/giveaways/{id}/" + a.path, Summary: a.summary,
		}, func(ctx context.Context, in *idInput) (*giveawayOutput, error) {
			c, err := caller(ctx)
			if err != nil {
				return nil, err
			}

			v, err := a.act(ctx, c.User.ID, in.ID)

			return o.giveaway(ctx, v, err)
		})
	}
}

func (o *Operations) listGiveaways(ctx context.Context, in *pageInput) (*listOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	mine, review, err := o.reader.List(ctx, o.tg, c.User.ID, in.Offset, in.Limit)
	if err != nil {
		return nil, o.apiErr(ctx, err)
	}

	out := &listOutput{}
	out.Body.Giveaways, out.Body.Total, out.Body.Review = toBodies(mine.Views), mine.Total, toBodies(review)
	out.Body.Channels = o.toListChannels(o.reader.Channels(ctx, o.tg, mine.Views, review))

	return out, nil
}

func (o *Operations) listFinished(ctx context.Context, in *pageInput) (*archiveOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	page, err := o.reader.Archived(ctx, c.User.ID, in.Offset, in.Limit)
	if err != nil {
		return nil, o.apiErr(ctx, err)
	}

	out := &archiveOutput{}
	out.Body.Giveaways, out.Body.Total = toBodies(page.Views), page.Total
	out.Body.Channels = o.toListChannels(o.reader.Channels(ctx, o.tg, page.Views))

	return out, nil
}

func (o *Operations) getGiveaway(ctx context.Context, in *idInput) (*giveawayDetailsOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	v, err := o.reader.Get(ctx, c.User.ID, in.ID)
	if err != nil {
		return nil, o.apiErr(ctx, err)
	}

	out := &giveawayDetailsOutput{}
	out.Body.Giveaway = toBody(v)

	if cs := o.toListChannels(o.reader.Channels(ctx, o.tg, []giveaway.View{v})); len(cs) > 0 {
		out.Body.Channel = cs[0]
	}

	return out, nil
}

func (o *Operations) createGiveaway(ctx context.Context, in *createInput) (*giveawayOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	f := giveaway.Fields{
		PostLink: in.Body.PostLink, EndsAt: in.Body.EndsAt, Winners: in.Body.Winners, ButtonText: in.Body.ButtonText,
	}
	v, err := o.writer.Create(ctx, o.tg, c.User.ID, c.Lang, f)

	return o.giveaway(ctx, v, err)
}

func (o *Operations) editGiveaway(ctx context.Context, in *editInput) (*giveawayOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	changes := giveaway.Changes{EndsAt: in.Body.EndsAt, Winners: in.Body.Winners, ButtonText: in.Body.ButtonText}
	v, err := o.writer.Edit(ctx, o.tg, c.User.ID, in.ID, changes)

	return o.giveaway(ctx, v, err)
}

func (o *Operations) moveGiveaway(ctx context.Context, in *moveInput) (*giveawayOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	v, err := o.writer.Move(ctx, o.tg, c.User.ID, in.ID, in.Body.PostLink)

	return o.giveaway(ctx, v, err)
}

// giveaway returns v, or the API error for err.
func (o *Operations) giveaway(ctx context.Context, v giveaway.View, err error) (*giveawayOutput, error) {
	if err != nil {
		return nil, o.apiErr(ctx, err)
	}

	return &giveawayOutput{Body: toBody(v)}, nil
}

// apiErr turns a domain error into an API error. Anything but a giveaway.AppError is a server
// failure: it is logged and the client gets "internal".
func (o *Operations) apiErr(ctx context.Context, err error) error {
	var appErr *giveaway.AppError
	if !errors.As(err, &appErr) {
		o.log.ErrorContext(ctx, "mini app request failed", "error", err)

		return problem(http.StatusInternalServerError, CodeInternal)
	}

	p := problem(appStatus(appErr.Code), appErr.Code)
	p.Field = appErr.Field

	return p
}

// appStatus is the HTTP status of a giveaway problem code.
func appStatus(code string) int {
	switch code {
	case giveaway.CodeNotFound:
		return http.StatusNotFound
	case giveaway.CodeNotYours, giveaway.CodeNotAdmin:
		return http.StatusForbidden
	case giveaway.CodeInvalidPostLink, giveaway.CodeInvalidEnd, giveaway.CodeEndInPast,
		giveaway.CodeInvalidWinners, giveaway.CodeInvalidButtonText:
		return http.StatusUnprocessableEntity
	case giveaway.CodeFailed:
		return http.StatusInternalServerError
	default: // finished, stale, due, exists, taken, other channel, bot rights, post gone, cannot message
		return http.StatusConflict
	}
}

// caller returns the user Auth verified, or 401.
func caller(ctx context.Context) (Caller, error) {
	c, ok := CallerFrom(ctx)
	if !ok {
		return Caller{}, problem(http.StatusUnauthorized, CodeUnauthorized)
	}

	return c, nil
}
