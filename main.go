// Command bot runs the Telegram bot for the @read_wine channel.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-telegram/bot"

	"github.com/alexander-danilenko/read_wine_bot/internal/config"
	"github.com/alexander-danilenko/read_wine_bot/internal/greeting"
	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
	"github.com/alexander-danilenko/read_wine_bot/internal/language"
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

	log := slog.Default()

	// bot.New calls getMe, so a wrong or revoked token stops the bot here.
	b, err := bot.New(
		cfg.TelegramBotToken,
		bot.WithMessageTextHandler(
			greeting.Command,
			bot.MatchTypeCommandStartOnly,
			greeting.Handler(texts, log),
		),
		bot.WithMessageTextHandler(
			language.Command,
			bot.MatchTypeCommandStartOnly,
			language.CommandHandler(texts, log),
		),
		bot.WithCallbackQueryDataHandler(
			language.CallbackPrefix,
			bot.MatchTypePrefix,
			language.ButtonHandler(texts, log),
		),
	)
	if err != nil {
		return fmt.Errorf("connect to telegram: %w", err)
	}

	slog.InfoContext(ctx, "bot started")
	b.Start(ctx)
	slog.InfoContext(ctx, "bot stopped")

	return nil
}
