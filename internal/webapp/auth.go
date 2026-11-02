package webapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// Why initData is checked here and not with go-telegram's ValidateWebappRequest: see
// docs/adr/0009-mini-app-api-trusts-only-initdata.md.
const (
	// MaxInitDataAge is how long Telegram's signed launch data stays valid. An admin may keep
	// the app open for a while, so it is longer than the usual hour; after it the app asks the
	// user to open it again from the bot.
	MaxInitDataAge = 24 * time.Hour
	// maxClockSkew tolerates a launch time slightly ahead of this server's clock.
	maxClockSkew = time.Minute
	// authScheme prefixes initData in the Authorization header.
	authScheme = "tma "
)

// Reasons Verify rejects initData. The client sees only "unauthorized"; the log names the reason.
var (
	ErrNoInitData        = errors.New("no Mini App launch data")
	ErrInitDataMalformed = errors.New("launch data is malformed")
	ErrInitDataSignature = errors.New("launch data signature does not match")
	ErrInitDataExpired   = errors.New("launch data is too old or from the future")
	ErrInitDataNoUser    = errors.New("launch data has no user")
)

// Verify checks Telegram's signature on initData (the Mini App launch data) with the bot token
// and returns the user it names. Telegram signs every field except hash: the key is
// HMAC-SHA256 of the token under "WebAppData", and the message is the fields sorted by name as
// key=value lines. The signature field (for third parties without the token) stays in the
// message like any other field.
func Verify(initData, botToken string, now time.Time) (models.User, error) {
	if initData == "" {
		return models.User{}, ErrNoInitData
	}

	fields, err := checkSignature(initData, botToken)
	if err != nil {
		return models.User{}, err
	}

	authDate, err := strconv.ParseInt(fields.Get("auth_date"), 10, 64)
	if err != nil {
		return models.User{}, ErrInitDataMalformed
	}

	if age := now.Sub(time.Unix(authDate, 0)); age > MaxInitDataAge || age < -maxClockSkew {
		return models.User{}, ErrInitDataExpired
	}

	var u models.User
	if err := json.Unmarshal([]byte(fields.Get("user")), &u); err != nil || u.ID == 0 {
		return models.User{}, ErrInitDataNoUser
	}

	return u, nil
}

// checkSignature parses initData and checks its hash field against the other fields.
func checkSignature(initData, botToken string) (url.Values, error) {
	fields, err := url.ParseQuery(initData)
	if err != nil {
		return nil, ErrInitDataMalformed
	}

	hash, err := hex.DecodeString(fields.Get("hash"))
	if err != nil || len(hash) != sha256.Size {
		return nil, ErrInitDataMalformed
	}

	lines := make([]string, 0, len(fields))
	for key, values := range fields {
		if len(values) != 1 {
			return nil, ErrInitDataMalformed
		}

		if key != "hash" {
			lines = append(lines, key+"="+values[0])
		}
	}

	slices.Sort(lines)

	if !hmac.Equal(hash, sign(botToken, strings.Join(lines, "\n"))) {
		return nil, ErrInitDataSignature
	}

	return fields, nil
}

// sign returns Telegram's initData signature of dataCheck.
func sign(botToken, dataCheck string) []byte {
	key := hmac.New(sha256.New, []byte("WebAppData"))
	key.Write([]byte(botToken))

	mac := hmac.New(sha256.New, key.Sum(nil))
	mac.Write([]byte(dataCheck))

	return mac.Sum(nil)
}

// Caller is the verified Telegram user behind an API request.
type Caller struct {
	User models.User
	Lang string // the language the bot uses for this user: one of the bot's locales
}

type callerKey struct{}

// CallerFrom returns the caller that Auth verified for the request.
func CallerFrom(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerKey{}).(Caller)

	return c, ok
}

// Profiles saves a user's profile. *user.Store implements it.
type Profiles interface {
	Record(ctx context.Context, p user.Profile) error
}

// Auth lets a request through only with valid launch data in the Authorization header. Before
// the handler runs it saves the user's profile (a giveaway row references its creator in the
// users table, and Mini App users skip the bot's update middleware) and resolves their language.
type Auth struct {
	botToken string
	profiles Profiles
	langs    language.Reader
	locales  []string
	log      *slog.Logger
	now      func() time.Time
}

// NewAuth checks launch data against botToken. locales are the bot's languages.
func NewAuth(botToken string, profiles Profiles, langs language.Reader, locales []string, log *slog.Logger) *Auth {
	return &Auth{botToken: botToken, profiles: profiles, langs: langs, locales: locales, log: log, now: time.Now}
}

// Wrap returns next behind the check.
func (a *Auth) Wrap(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		initData, ok := strings.CutPrefix(r.Header.Get("Authorization"), authScheme)
		if !ok {
			initData = ""
		}

		u, err := Verify(initData, a.botToken, a.now())
		if err != nil {
			a.log.InfoContext(ctx, "reject mini app request", "path", r.URL.Path, "reason", err)
			writeProblem(w, http.StatusUnauthorized, CodeUnauthorized)

			return
		}

		if err := a.profiles.Record(ctx, user.ProfileOf(&u)); err != nil {
			a.log.ErrorContext(ctx, "record mini app user", "user_id", u.ID, "error", err)
			writeProblem(w, http.StatusInternalServerError, CodeInternal)

			return
		}

		lang := language.UserLanguage(ctx, a.langs, a.log, &u)
		caller := Caller{User: u, Lang: a.locale(lang)}
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, callerKey{}, caller)))
	}
}

// locale maps a language code such as "en-US" to one of the bot's locales, as i18n.Text does.
func (a *Auth) locale(lang string) string {
	base, _, _ := strings.Cut(lang, "-")
	if slices.Contains(a.locales, base) {
		return base
	}

	return i18n.DefaultLang
}

// Error codes the API returns in the "code" field of an error body. The app maps each to its
// own text.
const (
	CodeUnauthorized = "unauthorized"
	CodeInternal     = "internal"
)

// writeProblem writes an RFC 9457 problem body, the shape every API error has.
func writeProblem(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	body := fmt.Sprintf(`{"status":%d,"title":%q,"code":%q}`, status, http.StatusText(status), code)
	_, _ = w.Write([]byte(body)) //nolint:errcheck // the client went away; nothing to do
}
