// Command bot runs the Telegram bot for the @read_wine channel.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/go-telegram/bot"
	"gorm.io/gorm"

	"github.com/alexander-danilenko/read_wine_bot/internal/config"
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

	// An unreachable database or a failed migration stops the bot here.
	conn, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := conn.DB()
	if err != nil {
		return fmt.Errorf("get database connection pool: %w", err)
	}
	defer sqlDB.Close()

	log := slog.Default()

	// bot.New calls getMe, so a wrong or revoked token stops the bot here.
	b, err := bot.New(cfg.TelegramBotToken, handlers(conn, texts, log)...)
	if err != nil {
		return fmt.Errorf("connect to telegram: %w", err)
	}

	slog.InfoContext(ctx, "bot started")
	b.Start(ctx)
	slog.InfoContext(ctx, "bot stopped")

	return nil
}

// handlers wires every update the bot handles to the package that handles it.
func handlers(conn *gorm.DB, texts *i18n.Catalog, log *slog.Logger) []bot.Option {
	users := user.NewStore(conn)
	giveaways := giveaway.NewService(giveaway.NewStore(conn), texts, log)

	return slices.Concat([]bot.Option{
		// Runs before every handler: keeps users' names current and leaves any unfinished
		// setup on a command or menu button.
		bot.WithMiddlewares(user.Middleware(users, menu.IsLabel(texts), log)),
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
		// A forwarded post matches no handler above, so it lands here.
		bot.WithDefaultHandler(giveaway.ForwardHandler(giveaways, texts, users, log)),
	}, menuHandlers(texts, users, log))
}

// menuHandlers wires the menu buttons. A button sends its label as text, so each label in
// each language gets its own exact-match handler.
func menuHandlers(texts *i18n.Catalog, users *user.Store, log *slog.Logger) []bot.Option {
	giveawayLabels, languageLabels := texts.All(i18n.MenuGiveaway), texts.All(i18n.MenuLanguage)
	opts := make([]bot.Option, 0, len(giveawayLabels)+len(languageLabels))

	for _, label := range giveawayLabels {
		opts = append(
			opts,
			bot.WithMessageTextHandler(label, bot.MatchTypeExact, giveaway.CommandHandler(texts, users, log)),
		)
	}

	for _, label := range languageLabels {
		opts = append(
			opts,
			bot.WithMessageTextHandler(label, bot.MatchTypeExact, language.PromptHandler(texts, users, log)),
		)
	}

	return opts
}
