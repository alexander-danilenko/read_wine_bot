package webapp

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Operations implements the API. Its fields are the domain services the operations call.
type Operations struct{}

// Register adds every operation to api.
func (o *Operations) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getMe",
		Method:      http.MethodGet,
		Path:        "/api/me",
		Summary:     "Who is using the app and in which language",
	}, o.me)
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
