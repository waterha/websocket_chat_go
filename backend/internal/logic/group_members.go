package logic

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

// RemoveMember lets an owner or admin remove another member, and lets any member
// remove themselves (leave). The owner can never be removed this way: ownership
// has to be transferred first, so a group always keeps an owner.
func (chat *Chat) RemoveMember(ctx context.Context, userID, conversationID, targetID string) error {
	if targetID == "" {
		return model.Fail(400, "invalid_members")
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	callerRole, err := chat.roleOf(ctx, tx, userID, conversationID)
	if err != nil {
		return err
	}
	var targetRole string
	err = tx.QueryRow(ctx, "SELECT role FROM conversation_members WHERE conversation_id=$1 AND user_id=$2", conversationID, targetID).Scan(&targetRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Fail(404, "not_found")
	}
	if err != nil {
		return err
	}
	leaving := targetID == userID
	if !leaving && callerRole != model.RoleOwner && callerRole != model.RoleAdmin {
		return model.Fail(403, "forbidden")
	}
	// An admin may not remove the owner or another admin; only the owner may.
	if callerRole == model.RoleAdmin && (targetRole == model.RoleOwner || targetRole == model.RoleAdmin) {
		return model.Fail(403, "forbidden")
	}
	if targetRole == model.RoleOwner {
		return model.Fail(403, "owner_must_transfer")
	}
	var targetName, callerName string
	if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", targetID).Scan(&targetName); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", userID).Scan(&callerName); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM conversation_members WHERE conversation_id=$1 AND user_id=$2", conversationID, targetID); err != nil {
		return err
	}
	notice := callerName + "将" + targetName + "移出群聊"
	if leaving {
		notice = targetName + "退出了群聊"
	}
	if err = chat.systemNotice(ctx, tx, conversationID, userID, notice); err != nil {
		return err
	}
	if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetRole promotes a member to admin or demotes an admin back to member. Only
// the owner may do this, and the owner role itself cannot be assigned here.
func (chat *Chat) SetRole(ctx context.Context, userID, conversationID, targetID, role string) error {
	if role != model.RoleAdmin && role != model.RoleMember {
		return model.Fail(400, "invalid_role")
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	callerRole, err := chat.roleOf(ctx, tx, userID, conversationID)
	if err != nil {
		return err
	}
	if callerRole != model.RoleOwner {
		return model.Fail(403, "forbidden")
	}
	var targetRole string
	err = tx.QueryRow(ctx, "SELECT role FROM conversation_members WHERE conversation_id=$1 AND user_id=$2", conversationID, targetID).Scan(&targetRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Fail(404, "not_found")
	}
	if err != nil {
		return err
	}
	if targetRole == model.RoleOwner {
		return model.Fail(400, "invalid_role")
	}
	if targetRole == role {
		return tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, "UPDATE conversation_members SET role=$1 WHERE conversation_id=$2 AND user_id=$3", role, conversationID, targetID); err != nil {
		return err
	}
	var targetName string
	if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", targetID).Scan(&targetName); err != nil {
		return err
	}
	notice := targetName + "被设为管理员"
	if role == model.RoleMember {
		notice = targetName + "被取消管理员"
	}
	if err = chat.systemNotice(ctx, tx, conversationID, userID, notice); err != nil {
		return err
	}
	if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// TransferOwnership hands the owner role to another member and demotes the
// previous owner to admin, keeping exactly one owner for the group.
func (chat *Chat) TransferOwnership(ctx context.Context, userID, conversationID, targetID string) error {
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Permission is checked before the target, so a non-owner always learns they
	// are forbidden rather than getting a misleading argument error.
	callerRole, err := chat.roleOf(ctx, tx, userID, conversationID)
	if err != nil {
		return err
	}
	if callerRole != model.RoleOwner {
		return model.Fail(403, "forbidden")
	}
	if targetID == userID || targetID == "" {
		return model.Fail(400, "invalid_members")
	}
	var targetName string
	err = tx.QueryRow(ctx, `SELECT u.name FROM users u JOIN conversation_members cm ON cm.user_id=u.id
		WHERE cm.conversation_id=$1 AND cm.user_id=$2`, conversationID, targetID).Scan(&targetName)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Fail(404, "not_found")
	}
	if err != nil {
		return err
	}
	// Demote before promoting so the partial unique owner index is never violated.
	if _, err = tx.Exec(ctx, "UPDATE conversation_members SET role='admin' WHERE conversation_id=$1 AND user_id=$2", conversationID, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE conversation_members SET role='owner' WHERE conversation_id=$1 AND user_id=$2", conversationID, targetID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE conversations SET created_by=$1 WHERE id=$2", targetID, conversationID); err != nil {
		return err
	}
	if err = chat.systemNotice(ctx, tx, conversationID, userID, targetName+"成为新的群主"); err != nil {
		return err
	}
	if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// maxMemberAliasRunes bounds the group card a member can carry.
const maxMemberAliasRunes = 32

// SetMemberAlias sets the group card of one member. Members may rename
// themselves; admins and the owner may rename anyone below their own rank.
func (chat *Chat) SetMemberAlias(ctx context.Context, callerID, conversationID, targetID, alias string) error {
	alias = strings.TrimSpace(alias)
	if targetID == "" || len([]rune(alias)) > maxMemberAliasRunes {
		return model.Fail(400, "invalid_member_alias")
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	callerRole, err := chat.roleOf(ctx, tx, callerID, conversationID)
	if err != nil {
		return err
	}
	if targetID != callerID && callerRole == model.RoleMember {
		return model.Fail(403, "forbidden")
	}
	tag, err := tx.Exec(ctx, "UPDATE conversation_members SET alias=$1 WHERE conversation_id=$2 AND user_id=$3", alias, conversationID, targetID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.Fail(404, "not_found")
	}
	if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
