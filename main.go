// Command bot runs the Telegram bot for the @read_wine channel.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/signal"
	"slices"
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
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

// version is the bot's semantic version (semver.org), bumped by every commit that changes the
// bot (see "Version" in CLAUDE.md). The startup log and the /start reply show it, so a deploy
// can be checked.
const version = "0.15.3"

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
	opts := append(handlers(users, store, reviewer, texts, kyiv, log), bot.WithAllowedUpdates(allowedUpdates()))

	b, err := bot.New(cfg.TelegramBotToken, opts...)
	if err != nil {
		return fmt.Errorf("connect to telegram: %w", err)
	}

	if err := registerChannelLog(ctx, b, channel.NewStore(conn), users, log); err != nil {
		return err
	}

	if err := command.Register(ctx, b, texts); err != nil {
		return fmt.Errorf("register commands: %w", err)
	}

	drawer := newDrawer(store, users, reviewer, texts, log)

	// Regular tasks. They share ctx with the bot, so the same signal stops them all. Waiting
	// for them before returning keeps the deferred database close from cutting a run short.
	var wg sync.WaitGroup
	wg.Go(func() { cron.Every(ctx, time.Minute, func(ctx context.Context) { drawer.DrawDue(ctx, b) }) })

	slog.InfoContext(ctx, "bot started", "version", version)
	b.Start(ctx)
	wg.Wait()
	slog.InfoContext(ctx, "bot stopped")

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

// newDrawer returns the winner draw, which sends the results to the creator for review.
func newDrawer(
	store *giveaway.Store, users *user.Store, reviewer *giveaway.Reviewer, texts *i18n.Catalog, log *slog.Logger,
) *giveaway.Drawer {
	kb := keyboard.NewTracker(users, log)
	notifier := giveaway.NewReviewNotifier(reviewer, users, texts, kb, log)

	return giveaway.NewDrawer(store, giveaway.AcceptAll, rand.Shuffle, notifier, time.Now, log)
}

// newReviewer returns the results review rules. A replacement winner is picked with the
// draw's filter and shuffle.
func newReviewer(
	store *giveaway.Store, channels *channel.Store, texts *i18n.Catalog, log *slog.Logger,
) *giveaway.Reviewer {
	return giveaway.NewReviewer(store, store, store, channels, giveaway.AcceptAll, rand.Shuffle, texts, time.Now, log)
}

// handlers wires every update the bot handles to the package that handles it.
func handlers(
	users *user.Store,
	store *giveaway.Store,
	reviewer *giveaway.Reviewer,
	texts *i18n.Catalog,
	kyiv *time.Location,
	log *slog.Logger,
) []bot.Option {
	giveaways := giveaway.NewService(store, store, users, texts, kyiv, time.Now, log)
	editor := giveaway.NewEditor(giveaways, store, store, store, store, store, store)
	kb := keyboard.NewTracker(users, log)

	return slices.Concat([]bot.Option{
		// Runs before every handler: records every sender's profile and its revisions, and
		// leaves any unfinished setup on a command.
		bot.WithMiddlewares(user.Middleware(users, kb, log)),
		// Before the greeting: a bot link sends /start with a payload, which the greeting
		// would also match, and the first matching handler wins.
		bot.WithMessageTextHandler(
			giveaway.ReviewLinkPrefix,
			bot.MatchTypePrefix,
			giveaway.ReviewLinkHandler(reviewer, texts, users, kb, log),
		),
		bot.WithMessageTextHandler(
			giveaway.FieldLinkPrefix,
			bot.MatchTypePrefix,
			giveaway.FieldLinkHandler(editor, texts, users, kb, log),
		),
		bot.WithMessageTextHandler(
			giveaway.EditLinkPrefix,
			bot.MatchTypePrefix,
			giveaway.EditLinkHandler(editor, texts, users, kb, log),
		),
		bot.WithMessageTextHandler(
			command.Start,
			bot.MatchTypeCommandStartOnly,
			command.StartHandler(texts, users, version, log),
		),
		bot.WithCallbackQueryDataHandler(
			language.CallbackPrefix,
			bot.MatchTypePrefix,
			language.ButtonHandler(texts, users, kb, log),
		),
		bot.WithCallbackQueryDataHandler(
			giveaway.JoinCallback,
			bot.MatchTypeExact,
			giveaway.JoinHandler(giveaways, texts, users, log),
		),
		bot.WithCallbackQueryDataHandler(
			giveaway.SetupCallbackPrefix,
			bot.MatchTypePrefix,
			giveaway.SetupCallbackHandler(giveaways, texts, users, kb, log),
		),
		bot.WithCallbackQueryDataHandler(
			giveaway.ReviewCallbackPrefix,
			bot.MatchTypePrefix,
			giveaway.ReviewCallbackHandler(reviewer, texts, users, kb, log),
		),
		bot.WithCallbackQueryDataHandler(
			giveaway.EditCallbackPrefix,
			bot.MatchTypePrefix,
			giveaway.EditCallbackHandler(editor, texts, users, kb, log),
		),
		// Forwarded posts and typed setup and edit values match no handler above, so they
		// land here.
		bot.WithDefaultHandler(giveaway.MessageHandler(giveaways, editor, texts, users, kb, log)),
	}, menuHandlers(texts, users, kb, giveaways, editor, log))
}

// menuHandlers wires the commands of the command menu and the page buttons of the lists.
func menuHandlers(
	texts *i18n.Catalog,
	users *user.Store,
	kb *keyboard.Tracker,
	giveaways *giveaway.Service,
	editor *giveaway.Editor,
	log *slog.Logger,
) []bot.Option {
	handlers := map[string]bot.HandlerFunc{
		command.New:          giveaway.StartHandler(giveaways, texts, users, kb, log),
		command.Edit:         giveaway.EditStartHandler(editor, texts, users, kb, log),
		command.List:         giveaway.ListHandler(editor, texts, users, kb, log),
		command.ListFinished: giveaway.ListFinishedHandler(editor, texts, users, kb, log),
		command.Language:     language.PromptHandler(texts, users, kb, log),
		command.Help:         command.HelpHandler(texts, users, log),
		command.Cancel:       command.CancelHandler(texts, users, log),
	}

	pageButtons := []bot.Option{
		bot.WithCallbackQueryDataHandler(
			giveaway.ListCallbackPrefix,
			bot.MatchTypePrefix,
			giveaway.ListCallbackHandler(editor, texts, users, kb, log),
		),
		bot.WithCallbackQueryDataHandler(
			giveaway.FinishedCallbackPrefix,
			bot.MatchTypePrefix,
			giveaway.FinishedCallbackHandler(editor, texts, users, kb, log),
		),
	}

	opts := make([]bot.Option, 0, len(pageButtons)+len(handlers))
	opts = append(opts, pageButtons...)

	for name, h := range handlers {
		opts = append(opts, bot.WithMessageTextHandler(name, bot.MatchTypeCommandStartOnly, h))
	}

	return opts
}
