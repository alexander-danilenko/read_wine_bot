package webapp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
)

// LanguageSaver saves the language a user picked. *user.Store implements it.
type LanguageSaver interface {
	SetLanguage(ctx context.Context, telegramID int64, lang string) error
}

// meBody is who uses the app, and the links that add the bot to a channel.
type meBody struct {
	Language    string `doc:"The language to show the app in"                json:"language"`
	FirstName   string `doc:"The user's first name"                          json:"first_name"`
	AddLink     string `doc:"Adds the bot to a channel with giveaway rights" json:"add_link"`
	PartnerLink string `doc:"Adds the bot to a partner channel"              json:"partner_link"`
}

// TransformSchema lists the languages as an enum in the spec, so the generated client knows them.
// It is not an enum tag: a tag key on one field only makes golines and tagalign rewrite each
// other's alignment, and an unexported embedded struct would drop its fields from the schema.
func (meBody) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	if lang, ok := s.Properties["language"]; ok {
		lang.Enum = []any{"en", "uk"}
	}

	return s
}

type meOutput struct {
	Body meBody
}

type setLanguageInput struct {
	Body struct {
		Language string `doc:"The language to save" enum:"en,uk" json:"language"`
	}
}

type myChannelsOutput struct {
	Body struct {
		Channels []listChannelBody `doc:"Channels where both the bot and the caller are admins" json:"channels"`
	}
}

func (o *Operations) registerMe(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getMe",
		Method:      http.MethodGet,
		Path:        "/api/me",
		Summary:     "Who is using the app and in which language",
	}, o.me)
	huma.Register(api, huma.Operation{
		OperationID: "setLanguage",
		Method:      http.MethodPut,
		Path:        "/api/me/language",
		Summary:     "Save the language the bot and the app use for the caller",
	}, o.setLanguage)
	huma.Register(api, huma.Operation{
		OperationID: "listMyChannels",
		Method:      http.MethodGet,
		Path:        "/api/channels",
		Summary:     "Channels where both the bot and the caller are admins",
	}, o.myChannels)
}

func (o *Operations) me(ctx context.Context, _ *struct{}) (*meOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	return o.meFor(c, c.Lang), nil
}

func (o *Operations) meFor(c Caller, lang string) *meOutput {
	return &meOutput{Body: meBody{
		Language:    lang,
		FirstName:   c.User.FirstName,
		AddLink:     giveaway.AddToChannelLink(o.bot),
		PartnerLink: giveaway.AddToPartnerChannelLink(o.bot),
	}}
}

func (o *Operations) setLanguage(ctx context.Context, in *setLanguageInput) (*meOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	if err := o.langs.SetLanguage(ctx, c.User.ID, in.Body.Language); err != nil {
		return nil, fmt.Errorf("save language of user %d: %w", c.User.ID, err)
	}

	return o.meFor(c, in.Body.Language), nil
}

func (o *Operations) myChannels(ctx context.Context, _ *struct{}) (*myChannelsOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	cs, err := o.channels.MyChannels(ctx, o.tg, c.User.ID)
	if err != nil {
		return nil, fmt.Errorf("list channels of user %d: %w", c.User.ID, err)
	}

	out := &myChannelsOutput{}
	out.Body.Channels = o.toListChannels(cs)

	return out, nil
}
