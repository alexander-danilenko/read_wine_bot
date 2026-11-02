package webapp

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
)

// Error codes in the "code" field of every error body. The app shows its own text for each.
const (
	CodeUnauthorized = "unauthorized" // launch data missing, forged or expired: open the app again
	CodeInternal     = "internal"     // a server or Telegram failure: try again later
	CodeInvalid      = "invalid"      // the request does not match the API schema
)

// errorCodes lists every error code, for the spec's enum. Add new codes here.
func errorCodes() []string {
	return []string{
		CodeUnauthorized, CodeInternal, CodeInvalid,
		giveaway.CodeNotFound, giveaway.CodeNotYours, giveaway.CodeFinished, giveaway.CodeNotAdmin,
		giveaway.CodeBotNoRights, giveaway.CodePostGone, giveaway.CodeExists, giveaway.CodeTaken,
		giveaway.CodeInvalidPostLink, giveaway.CodeInvalidEnd, giveaway.CodeEndInPast, giveaway.CodeInvalidWinners,
		giveaway.CodeInvalidButtonText, giveaway.CodeOtherChannel, giveaway.CodeDue, giveaway.CodeStale,
		giveaway.CodeFailed,
	}
}

// APIError is the body of every error response (RFC 9457 problem details plus a code).
type APIError struct {
	Status int                 `doc:"HTTP status code"                                        json:"status"`
	Title  string              `doc:"HTTP status text"                                        json:"title"`
	Code   string              `doc:"What went wrong; the app shows its own text for it"      json:"code"`
	Field  string              `doc:"The request field the error concerns, when there is one" json:"field,omitempty"`
	Detail string              `doc:"Details for developers, not for users"                   json:"detail,omitempty"`
	Errors []*huma.ErrorDetail `doc:"Schema validation errors"                                json:"errors,omitempty"`
}

func (p *APIError) Error() string { return p.Code }

// GetStatus is the HTTP status of the response.
func (p *APIError) GetStatus() int { return p.Status }

// ContentType makes JSON error bodies application/problem+json.
func (p *APIError) ContentType(ct string) string {
	if ct == "application/json" {
		return "application/problem+json"
	}

	return ct
}

// TransformSchema lists the codes as an enum in the spec, so the generated client knows them.
func (p *APIError) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	if code, ok := s.Properties["code"]; ok {
		codes := errorCodes()

		code.Enum = make([]any, len(codes))
		for i, c := range codes {
			code.Enum[i] = c
		}
	}

	return s
}

// problem returns an error response with status and code.
func problem(status int, code string) *APIError {
	return &APIError{Status: status, Title: http.StatusText(status), Code: code}
}

// newHumaError turns Huma's own errors (schema validation, unparsable bodies) into an APIError.
// It has huma.NewError's signature, interface result included.
//
//nolint:ireturn // huma.NewError's signature
func newHumaError(status int, msg string, errs ...error) huma.StatusError {
	code := CodeInternal
	if status < http.StatusInternalServerError {
		code = CodeInvalid
	}

	p := problem(status, code)
	p.Detail = msg

	for _, err := range errs {
		if d, ok := err.(huma.ErrorDetailer); ok {
			p.Errors = append(p.Errors, d.ErrorDetail())
		} else if err != nil {
			p.Errors = append(p.Errors, &huma.ErrorDetail{Message: err.Error()})
		}
	}

	return p
}

// securityScheme names the Authorization header scheme in the spec.
const securityScheme = "telegramInitData"

// NewAPI returns the API on mux, with its operations at their full /api/... paths. Huma's own
// spec, docs and schema routes are off: the spec lives in the repo (web/openapi.json), and
// the deployed bot does not publish it.
func NewAPI(mux *http.ServeMux, version string) huma.API { //nolint:ireturn // huma.API is the library's handle
	huma.NewError = newHumaError

	config := huma.DefaultConfig("read_wine_bot Mini App API", version)
	config.OpenAPIPath = ""
	config.DocsPath = ""
	config.SchemasPath = ""
	config.CreateHooks = nil // no $schema links in bodies
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		securityScheme: {
			Type:        "apiKey",
			In:          "header",
			Name:        "Authorization",
			Description: "tma followed by Telegram.WebApp.initData",
		},
	}
	config.Security = []map[string][]string{{securityScheme: {}}}

	return humago.New(mux, config)
}

// writeProblem writes p for handlers outside Huma, such as Auth.
func writeProblem(w http.ResponseWriter, log *slog.Logger, r *http.Request, p *APIError) {
	body, err := json.Marshal(p)
	if err != nil {
		log.ErrorContext(r.Context(), "encode error body", "error", err)
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)

	if _, err := w.Write(body); err != nil {
		log.DebugContext(r.Context(), "write error body", "error", err)
	}
}
