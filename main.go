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

	"github.com/alexander-danilenko/read_wine_bot/internal/config"
	"github.com/alexander-danilenko/read_wine_bot/internal/cron"
	"github.com/alexander-danilenko/read_wine_bot/internal/db"
	"github.com/alexander-danilenko/read_wine_bot/internal/giveaway"
	"github.com/alexander-danilenko/read_wine_bot/internal/greeting"
	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
	"github.com/alexander-danilenko/read_wine_bot/internal/menu"
	"github.com/alexander-danilenko/read_wine_bot/internal/user"
)

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
	b, err := bot.New(cfg.TelegramBotToken, handlers(users, store, texts, kyiv, log)...)
	if err != nil {
		return fmt.Errorf("connect to telegram: %w", err)
	}

	drawer := giveaway.NewDrawer(store, giveaway.AcceptAll, rand.Shuffle, texts, time.Now, log)

	// Regular tasks. They share ctx with the bot, so the same signal stops them all. Waiting
	// for them before returning keeps the deferred database close from cutting a run short.
	var wg sync.WaitGroup
	wg.Go(func() { cron.Every(ctx, time.Minute, func(ctx context.Context) { drawer.DrawDue(ctx, b) }) })

	slog.InfoContext(ctx, "bot started")
	b.Start(ctx)
	wg.Wait()
	slog.InfoContext(ctx, "bot stopped")

	return nil
}

// handlers wires every update the bot handles to the package that handles it.
func handlers(
	users *user.Store, store *giveaway.Store, texts *i18n.Catalog, kyiv *time.Location, log *slog.Logger,
) []bot.Option {
	giveaways := giveaway.NewService(store, store, users, texts, kyiv, time.Now, log)
	editor := giveaway.NewEditor(giveaways, store, store)

	return slices.Concat([]bot.Option{
		// Runs before every handler: records every sender's profile and its revisions, and
		// leaves any unfinished setup on a command or menu button.
		bot.WithMiddlewares(user.Middleware(users, menu.IsLabel(texts), log)),
		// Before the greeting: an edit link sends /start with a payload, which the greeting
		// would also match, and the first matching handler wins.
		bot.WithMessageTextHandler(
			giveaway.EditLinkPrefix,
			bot.MatchTypePrefix,
			giveaway.EditLinkHandler(editor, texts, users, log),
		),
		bot.WithMessageTextHandler(
			greeting.Command,
			bot.MatchTypeCommandStartOnly,
			greeting.Handler(texts, users, log),
		),
		bot.WithCallbackQueryDataHandler(
			language.CallbackPrefix,
			bot.MatchTypePrefix,
			language.ButtonHandler(texts, users, log),
		),
		bot.WithCallbackQueryDataHandler(
			giveaway.JoinCallback,
			bot.MatchTypeExact,
			giveaway.JoinHandler(giveaways, texts, users, log),
		),
		bot.WithCallbackQueryDataHandler(
			giveaway.SetupCallbackPrefix,
			bot.MatchTypePrefix,
			giveaway.SetupCallbackHandler(giveaways, texts, users, log),
		),
		bot.WithCallbackQueryDataHandler(
			giveaway.EditCallbackPrefix,
			bot.MatchTypePrefix,
			giveaway.EditCallbackHandler(editor, texts, users, log),
		),
		// Forwarded posts and typed setup and edit values match no handler above, so they
		// land here.
		bot.WithDefaultHandler(giveaway.MessageHandler(giveaways, editor, texts, users, log)),
	}, menuHandlers(texts, users, giveaways, editor, log))
}

// menuHandlers wires the menu buttons. A button sends its label as text, so each label in
// each language gets its own exact-match handler.
func menuHandlers(
	texts *i18n.Catalog, users *user.Store, giveaways *giveaway.Service, editor *giveaway.Editor, log *slog.Logger,
) []bot.Option {
	return slices.Concat(
		labelHandlers(texts.All(i18n.MenuGiveaway), giveaway.StartHandler(giveaways, texts, users, log)),
		labelHandlers(texts.All(i18n.MenuEdit), giveaway.EditStartHandler(editor, texts, users, log)),
		labelHandlers(texts.All(i18n.MenuList), giveaway.ListHandler(editor, texts, users, log)),
		labelHandlers(texts.All(i18n.MenuLanguage), language.PromptHandler(texts, users, log)),
	)
}

// labelHandlers routes a message whose text is exactly one of labels to h.
func labelHandlers(labels []string, h bot.HandlerFunc) []bot.Option {
	opts := make([]bot.Option, 0, len(labels))
	for _, label := range labels {
		opts = append(opts, bot.WithMessageTextHandler(label, bot.MatchTypeExact, h))
	}

	return opts
}
