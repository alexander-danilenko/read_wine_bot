package giveaway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Role is what a user may do in the Mini App, read live from their member record in the
// primary channel on every request and never stored.
type Role string

// Roles.
const (
	RoleFull     Role = "full"      // the channel owner, or an admin with Post messages and Edit messages of others
	RoleReadOnly Role = "read_only" // any other admin of the primary channel: may look, not change
	RolePartner  Role = "partner"   // everyone else
)

// MemberReader reads a user's member record in a chat. BotAPI implements it.
type MemberReader interface {
	ChatMember(ctx context.Context, chatID, userID int64) (models.ChatMember, error)
}

// Role returns userID's role from their member record in the primary channel. Any failed
// lookup is an error, never partner, so a Telegram outage cannot downgrade an admin.
func (a *App) Role(ctx context.Context, tg MemberReader, userID int64) (Role, error) {
	return roleIn(ctx, tg, a.log, a.primary, userID)
}

// adminRole returns userID's role, or not_admin for a partner: only admins of the primary
// channel see its giveaways.
func (a *App) adminRole(ctx context.Context, tg MemberReader, userID int64) (Role, error) {
	role, err := a.Role(ctx, tg, userID)
	if err != nil {
		return "", err
	}

	if role == RolePartner {
		return "", &AppError{Code: CodeNotAdmin}
	}

	return role, nil
}

// roleIn returns userID's role from their member record in primary, the primary channel, as
// App.Role describes.
func roleIn(ctx context.Context, tg MemberReader, log *slog.Logger, primary, userID int64) (Role, error) {
	m, err := tg.ChatMember(ctx, primary, userID)
	if errors.Is(err, bot.ErrorForbidden) || errors.Is(err, bot.ErrorBadRequest) {
		log.ErrorContext(ctx, "bot cannot read the primary channel", "chat_id", primary, "error", err)
	}

	if err != nil {
		return "", fmt.Errorf("read user %d in the primary channel: %w", userID, err)
	}

	return roleOf(m), nil
}

// roleOf maps a member record in the primary channel to a role. The two rights a full admin
// needs are the ones the bot itself needs to run a giveaway.
func roleOf(m models.ChatMember) Role {
	switch m.Type {
	case models.ChatMemberTypeOwner:
		return RoleFull
	case models.ChatMemberTypeAdministrator:
		if canRunGiveaways(m) {
			return RoleFull
		}

		return RoleReadOnly
	case models.ChatMemberTypeMember, models.ChatMemberTypeRestricted,
		models.ChatMemberTypeLeft, models.ChatMemberTypeBanned:
		return RolePartner
	default:
		return RolePartner
	}
}

// extraRights returns the admin rights set on m beyond Manage chat, which every admin has, as
// the snake_case names the Bot API uses. It is never nil, and empty for a non-admin.
func extraRights(m models.ChatMember) []string {
	a := m.Administrator
	if a == nil {
		return []string{}
	}

	rights := []struct {
		name string
		set  bool
	}{
		{"can_delete_messages", a.CanDeleteMessages},
		{"can_manage_video_chats", a.CanManageVideoChats},
		{"can_restrict_members", a.CanRestrictMembers},
		{"can_promote_members", a.CanPromoteMembers},
		{"can_change_info", a.CanChangeInfo},
		{"can_invite_users", a.CanInviteUsers},
		{"can_post_messages", a.CanPostMessages},
		{"can_edit_messages", a.CanEditMessages},
		{"can_pin_messages", a.CanPinMessages},
		{"can_post_stories", a.CanPostStories},
		{"can_edit_stories", a.CanEditStories},
		{"can_delete_stories", a.CanDeleteStories},
		{"can_manage_topics", a.CanManageTopics},
		{"can_manage_direct_messages", a.CanManageDirectMessages},
		{"can_manage_tags", a.CanManageTags},
		{"can_send_welcome_messages", a.CanSendWelcomeMessages},
	}

	extra := []string{}

	for _, r := range rights {
		if r.set {
			extra = append(extra, r.name)
		}
	}

	return extra
}
