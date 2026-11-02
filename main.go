// Command bot runs the Telegram bot for the @read_wine channel.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata" // embeds the time zone database for Europe/Kyiv

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/alexander-danilenko/read_wine_bot/internal/channel"
	"github.com/alexander-danilenko/read_wine_bot/internal/command"
	"github.com/alexander-danilenko/read_wine_bot/internal/config"
	"github.com/alexander-danilenko/read_wine_bot/internal/cron"
	"github.com/alexander-danilenko/read_wine_bot/internal/db"
	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/keyboard"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
	"github.com/alexander-danilenko/read_wine_bot/internal/webapp"
)

// version is the bot's semantic version (semver.org), bumped by every commit that changes the
// bot (see "Version" in CLAUDE.md). The startup log and the /start reply show it, so a deploy
// can be checked.
const version = "2.7.2"

func main() {
	if err := run(); err != nil {
		slog.Error("bot failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(ctx, ".")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	static, err := webapp.NewStatic(webapp.Built())
	if err != nil {
		return fmt.Errorf("load mini app: %w", err)
	}

	// A broken locale file stops the bot here.
	texts, err := i18n.Load()
	if err != nil {
		return fmt.Errorf("load translations: %w", err)
	}

	// Giveaway end times are in Kyiv time. The time/tzdata import embeds the zone database,
	// so this works in a container without /usr/share/zoneinfo.
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		return fmt.Errorf("load Kyiv time zone: %w", err)
	}

	// An unreachable database or a failed migration stops the bot here.
	conn, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := conn.DB()
	if err != nil {
		return fmt.Errorf("get database connection pool: %w", err)
	}
	defer sqlDB.Close()

	log := slog.Default()
	users := user.NewStore(conn)
	store := giveaway.NewStore(conn)

	// bot.New calls getMe, so a wrong or revoked token stops the bot here.
	reviewer := newReviewer(store, channel.NewStore(conn), texts, log)
	giveaways, editor := newGiveawayRules(store, texts, kyiv, log)
	b, err := bot.New(cfg.TelegramBotToken, handlers(users, giveaways, texts, cfg.AppURL, log)...)
	if err != nil {
		return fmt.Errorf("connect to telegram: %w", err)
	}

	if err = setUpBot(ctx, b, channel.NewStore(conn), users, texts, cfg.AppURL, log); err != nil {
		return err
	}

	drawer := newDrawer(store, users, texts, cfg.AppURL, log)

	app := giveaway.NewApp(editor, reviewer, store)

	mux, err := newMiniApp(ctx, cfg.TelegramBotToken, b, static, app, users, texts, log)
	if err != nil {
		return err
	}

	return serve(ctx, stop, b, drawer, mux, cfg.ListenAddr)
}

// newMiniApp routes the Mini App: its static files and its API over the giveaway rules. Only
// getMe tells the bot's username, which the API's add links need. A bot's username cannot
// change while it runs, so it is read once here.
func newMiniApp(
	ctx context.Context, token string, b *bot.Bot, static *webapp.Static, app *giveaway.App,
	users *user.Store, texts *i18n.Catalog, log *slog.Logger,
) (*http.ServeMux, error) {
	me, err := b.GetMe(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the bot's username: %w", err)
	}

	tg := giveaway.NewBotAPI(b)
	photos := webapp.NewPhotos(token, tg, log)
	ops := webapp.NewOperations(tg, app, photos, users, me.Username, log)
	auth := webapp.NewAuth(token, users, users, texts.Langs(), log)

	return webapp.NewMux(static, auth, ops, photos, version), nil
}

// serve runs the bot, the regular tasks and the Mini App server until ctx is done. They share
// ctx, so the same signal stops them all, and a failing server stops the others through stop.
// Waiting for them before returning keeps the deferred database close from cutting a run short.
func serve(
	ctx context.Context, stop context.CancelFunc, b *bot.Bot, drawer *giveaway.Drawer, mux http.Handler, addr string,
) error {
	// A busy port stops the bot here.
	srv, err := webapp.Listen(ctx, addr, mux)
	if err != nil {
		return fmt.Errorf("start mini app server: %w", err)
	}

	var (
		wg     sync.WaitGroup
		srvErr error // set by the server's goroutine; read only after wg.Wait
	)

	wg.Go(func() { cron.Every(ctx, time.Minute, func(ctx context.Context) { drawer.DrawDue(ctx, b) }) })
	wg.Go(func() {
		if srvErr = srv.Run(ctx); srvErr != nil {
			stop()
		}
	})

	slog.InfoContext(ctx, "bot started", "version", version, "listen_addr", srv.Addr().String())
	b.Start(ctx)
	wg.Wait()
	slog.InfoContext(ctx, "bot stopped")

	if srvErr != nil {
		return fmt.Errorf("mini app server: %w", srvErr)
	}

	return nil
}

// setUpBot starts the channel log and tells Telegram what the bot offers.
func setUpBot(
	ctx context.Context, b *bot.Bot, audience *channel.Store, users *user.Store, texts *i18n.Catalog,
	appURL string, log *slog.Logger,
) error {
	if err := registerChannelLog(ctx, b, audience, users, log); err != nil {
		return err
	}

	return describeBot(ctx, b, texts, appURL)
}

// describeBot tells Telegram what the bot offers and needs: the command menu, the menu button
// that opens the Mini App, and the admin rights Telegram pre-selects when an admin adds the
// bot to a channel.
func describeBot(ctx context.Context, b *bot.Bot, texts *i18n.Catalog, appURL string) error {
	if err := command.Register(ctx, b, texts); err != nil {
		return fmt.Errorf("register commands: %w", err)
	}

	if err := command.SetMenuButton(ctx, b, texts, appURL); err != nil {
		return err //nolint:wrapcheck // SetMenuButton already says what failed
	}

	if err := giveaway.RequestRights(ctx, b); err != nil {
		return fmt.Errorf("request channel admin rights: %w", err)
	}

	return nil
}

// registerChannelLog wires the channel's audience log and the log of the channels where the
// bot is an admin, then checks the bot's status in every known channel. go-telegram/bot has
// no option for handlers matched by update type, so they are registered on the bot; they
// match no update the handlers from handlers take.
func registerChannelLog(
	ctx context.Context, b *bot.Bot, audience *channel.Store, users *user.Store, log *slog.Logger,
) error {
	b.RegisterHandlerMatchFunc(
		func(u *models.Update) bool { return u.ChatMember != nil },
		channel.MemberHandler(audience, users, log),
	)
	b.RegisterHandlerMatchFunc(
		func(u *models.Update) bool { return u.ChannelPost != nil },
		channel.PostHandler(audience, log),
	)
	b.RegisterHandlerMatchFunc(
		func(u *models.Update) bool { return u.MyChatMember != nil },
		channel.BotHandler(audience, log),
	)

	if err := channel.CheckBotChannels(ctx, b, audience, time.Now(), log); err != nil {
		return fmt.Errorf("check channels where the bot is an admin: %w", err)
	}

	return nil
}

// allowedUpdates lists the update types the bot asks Telegram for. Telegram sends chat_member
// only when it is listed, and a list replaces Telegram's default set, so it names every type a
// handler or user.Middleware reads. Add a type here before handling it.
func allowedUpdates() bot.AllowedUpdates {
	return bot.AllowedUpdates{
		models.AllowedUpdateMessage,
		models.AllowedUpdateEditedMessage,
		models.AllowedUpdateChannelPost,
		models.AllowedUpdateCallbackQuery,
		models.AllowedUpdateMyChatMember,
		models.AllowedUpdateChatMember,
		models.AllowedUpdateChatJoinRequest,
	}
}

// newDrawer returns the winner draw, which tells the creator that the results wait for review
// in the Mini App.
func newDrawer(
	store *giveaway.Store, users *user.Store, texts *i18n.Catalog, appURL string, log *slog.Logger,
) *giveaway.Drawer {
	notifier := giveaway.NewReviewNotifier(users, texts, keyboard.NewTracker(users, log), appURL, log)

	return giveaway.NewDrawer(store, giveaway.AcceptAll, rand.Shuffle, notifier, time.Now, log)
}

// newReviewer returns the results review rules. A replacement winner is picked with the
// draw's filter and shuffle.
func newReviewer(
	store *giveaway.Store, channels *channel.Store, texts *i18n.Catalog, log *slog.Logger,
) *giveaway.Reviewer {
	return giveaway.NewReviewer(store, store, store, channels, giveaway.AcceptAll, rand.Shuffle, texts, time.Now, log)
}

// newGiveawayRules returns the giveaway rules the join button and the Mini App share.
func newGiveawayRules(
	store *giveaway.Store, texts *i18n.Catalog, kyiv *time.Location, log *slog.Logger,
) (*giveaway.Service, *giveaway.Editor) {
	svc := giveaway.NewService(store, store, texts, kyiv, time.Now, log)

	return svc, giveaway.NewEditor(svc, store, store, store, store, store, store)
}

// handlers wires every update the bot handles to the package that handles it, and asks
// Telegram for those updates only. Giveaways are managed in the Mini App; the chat keeps
// /start, the commands that open the app's screens and the join button.
func handlers(
	users *user.Store, giveaways *giveaway.Service, texts *i18n.Catalog, appURL string, log *slog.Logger,
) []bot.Option {
	kb := keyboard.NewTracker(users, log)

	screens := command.ScreenHandlers(texts, users, kb, appURL, log)
	screenOpts := make([]bot.Option, 0, len(screens))

	for cmd, h := range screens {
		screenOpts = append(screenOpts, bot.WithMessageTextHandler(cmd, bot.MatchTypeCommandStartOnly, h))
	}

	return append([]bot.Option{
		// Runs before every handler: records every sender's profile and its revisions, and
		// deletes the message with buttons on a command.
		bot.WithMiddlewares(user.Middleware(users, kb, log)),
		bot.WithMessageTextHandler(
			command.Start,
			bot.MatchTypeCommandStartOnly,
			command.StartHandler(texts, users, kb, appURL, version, log),
		),
		bot.WithCallbackQueryDataHandler(
			giveaway.JoinCallback,
			bot.MatchTypeExact,
			giveaway.JoinHandler(giveaways, texts, users, log),
		),
		// Any other message gets no answer. Without this, go-telegram logs every unmatched update.
		bot.WithDefaultHandler(answerStaleButton(log)),
		bot.WithAllowedUpdates(allowedUpdates()),
	}, screenOpts...)
}

// answerStaleButton answers a tap on a button no handler matches, such as one left from the
// chat-managed giveaways of 1.x, so Telegram stops its spinner. Any other update is ignored.
func answerStaleButton(log *slog.Logger) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.CallbackQuery == nil {
			return
		}

		_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID})
		if err != nil {
			log.ErrorContext(ctx, "answer stale button", "user_id", update.CallbackQuery.From.ID, "error", err)
		}
	}
}
