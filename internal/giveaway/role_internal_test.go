package giveaway

import (
	"slices"
	"testing"

	"github.com/go-telegram/bot/models"
)

func admin(a models.ChatMemberAdministrator) models.ChatMember {
	return models.ChatMember{Type: models.ChatMemberTypeAdministrator, Administrator: &a}
}

func TestRoleOf(t *testing.T) {
	tests := []struct {
		name string
		m    models.ChatMember
		want Role
	}{
		{"owner", models.ChatMember{Type: models.ChatMemberTypeOwner, Owner: &models.ChatMemberOwner{}}, RoleFull},
		{
			"admin with post and edit",
			admin(models.ChatMemberAdministrator{CanPostMessages: true, CanEditMessages: true}),
			RoleFull,
		},
		{"admin with post only", admin(models.ChatMemberAdministrator{CanPostMessages: true}), RoleReadOnly},
		{"admin with edit only", admin(models.ChatMemberAdministrator{CanEditMessages: true}), RoleReadOnly},
		{"admin with neither", admin(models.ChatMemberAdministrator{CanManageChat: true}), RoleReadOnly},
		{
			"member",
			models.ChatMember{Type: models.ChatMemberTypeMember, Member: &models.ChatMemberMember{}},
			RolePartner,
		},
		{"left", models.ChatMember{Type: models.ChatMemberTypeLeft, Left: &models.ChatMemberLeft{}}, RolePartner},
		{
			"banned",
			models.ChatMember{Type: models.ChatMemberTypeBanned, Banned: &models.ChatMemberBanned{}},
			RolePartner,
		},
		{
			"restricted",
			models.ChatMember{Type: models.ChatMemberTypeRestricted, Restricted: &models.ChatMemberRestricted{}},
			RolePartner,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := roleOf(tt.m); got != tt.want {
				t.Errorf("roleOf() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtraRights(t *testing.T) {
	tests := []struct {
		name string
		m    models.ChatMember
		want []string
	}{
		{"manage chat only", admin(models.ChatMemberAdministrator{CanManageChat: true}), nil},
		{
			"manage chat, post and invite users",
			admin(models.ChatMemberAdministrator{CanManageChat: true, CanPostMessages: true, CanInviteUsers: true}),
			[]string{"can_invite_users", "can_post_messages"},
		},
		{
			"every right",
			admin(models.ChatMemberAdministrator{
				CanBeEdited: true, CanManageChat: true, CanDeleteMessages: true, CanManageVideoChats: true,
				CanRestrictMembers: true, CanPromoteMembers: true, CanChangeInfo: true, CanInviteUsers: true,
				CanPostMessages: true, CanEditMessages: true, CanPinMessages: true, CanPostStories: true,
				CanEditStories: true, CanDeleteStories: true, CanManageTopics: true,
				CanManageDirectMessages: true, CanManageTags: true, CanSendWelcomeMessages: true,
			}),
			[]string{
				"can_delete_messages", "can_manage_video_chats", "can_restrict_members", "can_promote_members",
				"can_change_info", "can_invite_users", "can_post_messages", "can_edit_messages",
				"can_pin_messages", "can_post_stories", "can_edit_stories", "can_delete_stories",
				"can_manage_topics", "can_manage_direct_messages", "can_manage_tags", "can_send_welcome_messages",
			},
		},
		{"member", models.ChatMember{Type: models.ChatMemberTypeMember, Member: &models.ChatMemberMember{}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extraRights(tt.m)
			slices.Sort(got)

			want := slices.Clone(tt.want)
			slices.Sort(want)

			if !slices.Equal(got, want) {
				t.Errorf("extraRights() = %v, want %v", got, want)
			}
		})
	}
}
