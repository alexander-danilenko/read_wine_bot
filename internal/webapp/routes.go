package webapp

import (
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
)

// Operations implements the API over the domain services it calls. The zero value only
// registers the operations, which is enough to render the spec.
type Operations struct {
	tg      giveaway.AppTelegram
	reader  GiveawayReader
	writer  GiveawayWriter
	actions GiveawayActions
	log     *slog.Logger
}

// NewOperations returns the API's operations. tg makes the Telegram calls the giveaway rules
// need; *giveaway.App implements reader, writer and actions.
func NewOperations(
	tg giveaway.AppTelegram, reader GiveawayReader, writer GiveawayWriter, actions GiveawayActions, log *slog.Logger,
) *Operations {
	return &Operations{tg: tg, reader: reader, writer: writer, actions: actions, log: log}
}

// Register adds every operation to api.
func (o *Operations) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getMe",
		Method:      http.MethodGet,
		Path:        "/api/me",
		Summary:     "Who is using the app and in which language",
	}, o.me)
	o.registerGiveaways(api)
}

// NewMux routes the Mini App: the API behind Auth, and the static files.
func NewMux(static http.Handler, auth *Auth, ops *Operations, version string) *http.ServeMux {
	apiMux := http.NewServeMux()
	ops.Register(NewAPI(apiMux, version))

	mux := http.NewServeMux()
	mux.Handle("/", static)
	mux.Handle("/api/", auth.Wrap(apiMux))

	return mux
}
