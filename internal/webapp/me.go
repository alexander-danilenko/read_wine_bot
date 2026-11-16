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

// meBody is who uses the app, what they may do in it, and the link that adds the bot to a
// partner channel.
type meBody struct {
	Language    string `doc:"The language to show the app in"                          json:"language"`
	FirstName   string `doc:"The user's first name"                                    json:"first_name"`
	Role        string `doc:"What the user may do, read live from the primary channel" json:"role"`
	PartnerLink string `doc:"Adds the bot to a partner channel"                        json:"partner_link"`
}

// TransformSchema lists the languages and roles as enums in the spec, so the generated client
// knows them. They are not enum tags: a tag key on one field only makes golines and tagalign
// rewrite each other's alignment, and an unexported embedded struct would drop its fields from
// the schema.
func (meBody) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	if lang, ok := s.Properties["language"]; ok {
		lang.Enum = []any{"en", "uk"}
	}

	if role, ok := s.Properties["role"]; ok {
		role.Enum = []any{giveaway.RoleFull, giveaway.RoleReadOnly, giveaway.RolePartner}
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
		Channels []listChannelBody `doc:"The primary channel for its admins; for a partner, the other channels where both they and the bot are admins" json:"channels"`
		Partners []listChannelBody `doc:"For a primary channel admin, every other channel where the bot is an admin; empty for a partner"              json:"partners"`
	}
}

func (o *Operations) registerMe(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getMe",
		Method:      http.MethodGet,
		Path:        "/api/me",
		Summary:     "Who is using the app, in which language and with which role",
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
		Summary:     "The channels where the bot is an admin, as the caller's role shows them",
	}, o.myChannels)
}

func (o *Operations) me(ctx context.Context, _ *struct{}) (*meOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	return o.meFor(ctx, c, c.Lang)
}

// meFor returns the me body of c in lang, with c's role read live from Telegram.
func (o *Operations) meFor(ctx context.Context, c Caller, lang string) (*meOutput, error) {
	role, err := o.roles.Role(ctx, o.tg, c.User.ID)
	if err != nil {
		return nil, fmt.Errorf("read role of user %d: %w", c.User.ID, err)
	}

	return &meOutput{Body: meBody{
		Language:    lang,
		FirstName:   c.User.FirstName,
		Role:        string(role),
		PartnerLink: giveaway.AddToPartnerChannelLink(o.bot),
	}}, nil
}

func (o *Operations) setLanguage(ctx context.Context, in *setLanguageInput) (*meOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	if err := o.langs.SetLanguage(ctx, c.User.ID, in.Body.Language); err != nil {
		return nil, fmt.Errorf("save language of user %d: %w", c.User.ID, err)
	}

	return o.meFor(ctx, c, in.Body.Language)
}

func (o *Operations) myChannels(ctx context.Context, _ *struct{}) (*myChannelsOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	lists, err := o.channels.MyChannels(ctx, o.tg, c.User.ID)
	if err != nil {
		return nil, fmt.Errorf("list channels of user %d: %w", c.User.ID, err)
	}

	out := &myChannelsOutput{}
	out.Body.Channels = o.toListChannels(lists.Mine)
	out.Body.Partners = o.toListChannels(lists.Partners)

	return out, nil
}
