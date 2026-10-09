package giveaway

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/i18n"
)

// StartResult is the outcome of an attempt to start a giveaway on a channel post.
type StartResult int

// Start outcomes, each answered with its own reply to the admin.
const (
	Created  StartResult = iota // button attached and giveaway saved
	Exists                      // a giveaway already runs on the post
	NotAdmin                    // the forwarder is not an admin of the post's channel
	NoRights                    // the bot cannot read the channel's admins or edit the post
	Failed                      // anything else went wrong
)

// JoinResult is the outcome of a tap on the join button.
type JoinResult int

// Join outcomes, each answered with its own popup.
const (
	Joined        JoinResult = iota // the user is now a participant
	AlreadyJoined                   // the user was a participant before this tap
	JoinFailed                      // the tap could not be recorded
)

// Storage finds and saves giveaways and participants. *Store implements it.
type Storage interface {
	Find(ctx context.Context, chatID, messageID int64) (Giveaway, error)
	Create(ctx context.Context, g *Giveaway) error
	Join(ctx context.Context, giveawayID uint, telegramID int64) (bool, error)
}

// Drafts saves the inputs of unfinished giveaway setups. *Store implements it.
type Drafts interface {
	SaveDraft(ctx context.Context, d *Draft) error
	Draft(ctx context.Context, userID int64) (Draft, error)
	DeleteDraft(ctx context.Context, userID int64) error
}

// Steps holds the setup step each user is at, as their pending command. *user.Store
// implements it.
type Steps interface {
	PendingCommand(ctx context.Context, telegramID int64) (string, error)
	SetPendingCommand(ctx context.Context, telegramID int64, command string) error
	MovePendingCommand(ctx context.Context, telegramID int64, from, to string) (bool, error)
}

// Telegram is what the rules need from the Telegram API.
type Telegram interface {
	// IsChannelAdmin reports whether the user is the creator or an administrator of the channel.
	IsChannelAdmin(ctx context.Context, chatID, userID int64) (bool, error)
	// CanEditPosts reports whether the bot may edit the channel's posts.
	CanEditPosts(ctx context.Context, chatID int64) (bool, error)
	// AttachButton puts a single join button with label under the channel post. A post that
	// already carries that button counts as success.
	AttachButton(ctx context.Context, chatID, messageID int64, label string) error
}

// Service holds the giveaway rules: who may start one, how the setup collects its inputs,
// and what a tap records.
type Service struct {
	store  Storage
	drafts Drafts
	steps  Steps
	texts  *i18n.Catalog
	kyiv   *time.Location   // end dates and times are typed and shown in Kyiv time
	now    func() time.Time // the clock; tests fix it
	log    *slog.Logger
}

// NewService returns a Service.
func NewService(
	store Storage, drafts Drafts, steps Steps,
	texts *i18n.Catalog, kyiv *time.Location, now func() time.Time, log *slog.Logger,
) *Service {
	return &Service{store: store, drafts: drafts, steps: steps, texts: texts, kyiv: kyiv, now: now, log: log}
}

// Start starts a giveaway on the channel post for the admin, with the button and popups in
// lang. tg is passed per call because handlers get the bot only when an update arrives.
// Only after the post check passes and the button is attached does it save the giveaway, so
// a saved giveaway always has its button.
func (s *Service) Start(ctx context.Context, tg Telegram, adminID, chatID, messageID int64, lang string) StartResult {
	log := s.log.With("chat_id", chatID, "message_id", messageID, "user_id", adminID)

	if result, ok := s.checkPost(ctx, tg, adminID, chatID, messageID); !ok {
		return result
	}

	err := tg.AttachButton(ctx, chatID, messageID, s.texts.Text(lang, i18n.GiveawayButton))
	if err != nil {
		log.WarnContext(ctx, "attach join button", "error", err)

		return NoRights
	}

	err = s.store.Create(ctx, &Giveaway{ChatID: chatID, MessageID: messageID, CreatedBy: adminID, Language: lang})
	if errors.Is(err, ErrExists) {
		return Exists
	}

	if err != nil {
		log.ErrorContext(ctx, "save giveaway", "error", err)

		return Failed
	}

	return Created
}

// Join records the user as a participant of the giveaway on the channel post. It also
// returns the giveaway's language for the popup, or "" if no giveaway runs on the post.
func (s *Service) Join(ctx context.Context, chatID, messageID, userID int64) (JoinResult, string) {
	log := s.log.With("chat_id", chatID, "message_id", messageID, "user_id", userID)

	g, err := s.store.Find(ctx, chatID, messageID)
	if err != nil {
		log.ErrorContext(ctx, "find giveaway for join", "error", err)

		return JoinFailed, ""
	}

	added, err := s.store.Join(ctx, g.ID, userID)
	if err != nil {
		log.ErrorContext(ctx, "join giveaway", "error", err)

		return JoinFailed, g.Language
	}

	if !added {
		return AlreadyJoined, g.Language
	}

	return Joined, g.Language
}

// checkPost checks, in order, that no giveaway runs on the post yet, that the admin is an
// admin of its channel and that the bot may edit its posts. It returns the failed outcome
// and false, or true when every check passes.
func (s *Service) checkPost(ctx context.Context, tg Telegram, adminID, chatID, messageID int64) (StartResult, bool) {
	log := s.log.With("chat_id", chatID, "message_id", messageID, "user_id", adminID)

	_, err := s.store.Find(ctx, chatID, messageID)
	if err == nil {
		return Exists, false
	}

	if !errors.Is(err, ErrNotFound) {
		log.ErrorContext(ctx, "find giveaway", "error", err)

		return Failed, false
	}

	isAdmin, err := tg.IsChannelAdmin(ctx, chatID, adminID)
	if err != nil {
		log.WarnContext(ctx, "check channel admin", "error", err)

		return NoRights, false
	}

	if !isAdmin {
		return NotAdmin, false
	}

	canEdit, err := tg.CanEditPosts(ctx, chatID)
	if err != nil || !canEdit {
		log.WarnContext(ctx, "check bot rights", "can_edit", canEdit, "error", err)

		return NoRights, false
	}

	return Created, true
}
