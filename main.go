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
const version = "0.5.2"

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
	opts := append(handlers(users, store, texts, kyiv, log), bot.WithAllowedUpdates(allowedUpdates()))

	b, err := bot.New(cfg.TelegramBotToken, opts...)
	if err != nil {
		return fmt.Errorf("connect to telegram: %w", err)
	}

	registerAudienceLog(b, channel.NewStore(conn), users, log)

	if err := command.Register(ctx, b, texts); err != nil {
		return fmt.Errorf("register commands: %w", err)
	}

	drawer := giveaway.NewDrawer(store, giveaway.AcceptAll, rand.Shuffle, texts, time.Now, log)

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

// registerAudienceLog wires the channel's audience log. go-telegram/bot has no option for
// handlers matched by update type, so they are registered on the bot; they match no update
// the handlers from handlers take.
func registerAudienceLog(b *bot.Bot, audience *channel.Store, users *user.Store, log *slog.Logger) {
	b.RegisterHandlerMatchFunc(
		func(u *models.Update) bool { return u.ChatMember != nil },
		channel.MemberHandler(audience, users, log),
	)
	b.RegisterHandlerMatchFunc(
		func(u *models.Update) bool { return u.ChannelPost != nil },
		channel.PostHandler(audience, log),
	)
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

// handlers wires every update the bot handles to the package that handles it.
func handlers(
	users *user.Store, store *giveaway.Store, texts *i18n.Catalog, kyiv *time.Location, log *slog.Logger,
) []bot.Option {
	giveaways := giveaway.NewService(store, store, users, texts, kyiv, time.Now, log)
	editor := giveaway.NewEditor(giveaways, store, store, store)
	kb := keyboard.NewTracker(users, log)

	return slices.Concat([]bot.Option{
		// Runs before every handler: records every sender's profile and its revisions, and
		// leaves any unfinished setup on a command.
		bot.WithMiddlewares(user.Middleware(users, kb, log)),
		// Before the greeting: an edit link sends /start with a payload, which the greeting
		// would also match, and the first matching handler wins.
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
			giveaway.EditCallbackPrefix,
			bot.MatchTypePrefix,
			giveaway.EditCallbackHandler(editor, texts, users, kb, log),
		),
		// Forwarded posts and typed setup and edit values match no handler above, so they
		// land here.
		bot.WithDefaultHandler(giveaway.MessageHandler(giveaways, editor, texts, users, kb, log)),
	}, menuHandlers(texts, users, kb, giveaways, editor, log))
}

// menuHandlers wires the commands of the command menu.
func menuHandlers(
	texts *i18n.Catalog,
	users *user.Store,
	kb *keyboard.Tracker,
	giveaways *giveaway.Service,
	editor *giveaway.Editor,
	log *slog.Logger,
) []bot.Option {
	handlers := map[string]bot.HandlerFunc{
		command.New:      giveaway.StartHandler(giveaways, texts, users, kb, log),
		command.Edit:     giveaway.EditStartHandler(editor, texts, users, kb, log),
		command.List:     giveaway.ListHandler(editor, texts, users, log),
		command.Language: language.PromptHandler(texts, users, kb, log),
		command.Cancel:   command.CancelHandler(texts, users, log),
	}

	opts := make([]bot.Option, 0, len(handlers))
	for name, h := range handlers {
		opts = append(opts, bot.WithMessageTextHandler(name, bot.MatchTypeCommandStartOnly, h))
	}

	return opts
}
