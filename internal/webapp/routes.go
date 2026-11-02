package webapp

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
)

// Operations implements the API over the domain services it calls. The zero value only
// registers the operations, which is enough to render the spec.
type Operations struct {
	tg            Telegram
	reader        GiveawayReader
	writer        GiveawayWriter
	actions       GiveawayActions
	reviewReader  ReviewReader
	reviewActions ReviewActions
	winners       WinnersSender
	channels      ChannelLister
	langs         LanguageSaver
	photos        *Photos
	bot           string // the bot's username, without the @, for the add links
	log           *slog.Logger
}

// Telegram is every Telegram call the operations make. giveaway.BotAPI implements it.
type Telegram interface {
	giveaway.AppTelegram
	giveaway.ReviewTelegramAll
}

// Domain is the giveaway rules the operations call. *giveaway.App implements every part.
type Domain interface {
	GiveawayReader
	GiveawayWriter
	GiveawayActions
	ReviewReader
	ReviewActions
	WinnersSender
	ChannelLister
}

// ChannelLister lists the caller's channels. *giveaway.App implements it.
type ChannelLister interface {
	MyChannels(ctx context.Context, tg giveaway.AdminChannelsTelegram, userID int64) ([]giveaway.Channel, error)
}

// NewOperations returns the API's operations over d. tg makes the Telegram calls, langs saves
// the language a user picks, and bot is the bot's username for the links that add it.
func NewOperations(
	tg Telegram, d Domain, photos *Photos, langs LanguageSaver, bot string, log *slog.Logger,
) *Operations {
	return &Operations{
		tg:            tg,
		reader:        d,
		writer:        d,
		actions:       d,
		reviewReader:  d,
		reviewActions: d,
		winners:       d,
		channels:      d,
		langs:         langs,
		photos:        photos,
		bot:           bot,
		log:           log,
	}
}

// Register adds every operation to api.
func (o *Operations) Register(api huma.API) {
	o.registerMe(api)
	o.registerGiveaways(api)
	o.registerReviews(api)
}

// NewMux routes the Mini App: the API behind Auth, and the static files.
func NewMux(static http.Handler, auth *Auth, ops *Operations, photos *Photos, version string) *http.ServeMux {
	apiMux := http.NewServeMux()
	ops.Register(NewAPI(apiMux, version))

	mux := http.NewServeMux()
	mux.Handle("/", static)
	mux.Handle(photoPath, photos)
	mux.Handle("/api/", auth.Wrap(finishWrites(apiMux)))

	return mux
}

// finishWrites lets a request that changes data run to the end even if the client goes away:
// a giveaway write is several Telegram and database steps that must not stop halfway.
func finishWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)

			return
		}

		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), writeTimeout)
		defer cancel()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
