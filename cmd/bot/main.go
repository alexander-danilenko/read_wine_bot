// Command bot runs the Telegram bot behind quick giveaways in the @read_wine channel.
package main

import (
	"log/slog"
	"os"

	"github.com/alexander-danilenko/read_wine_giveaway_bot/internal/config"
)

func main() {
	if _, err := config.Load(); err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	slog.Info("hello from read_wine_giveaway_bot")
}
