// Command bot runs the Telegram bot for the @read_wine channel.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/alexander-danilenko/read_wine_bot/internal/config"
)

func main() {
	ctx := context.Background()

	if _, err := config.Load(ctx, "."); err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	slog.Info("hello from read_wine_bot")
}
