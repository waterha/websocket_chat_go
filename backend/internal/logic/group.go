package logic

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

// roleOf returns the caller's role in a group, or 404 when the conversation is
// missing, not a group, or the caller is not a member. It locks the conversation
// row so the role cannot change underneath a governance operation.
func (chat *Chat) roleOf(ctx context.Context, tx pgx.Tx, userID, conversationID string) (string, error) {
	var kind, role string
	err := tx.QueryRow(ctx, `SELECT c.kind,cm.role FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id
		WHERE c.id=$1 AND cm.user_id=$2 FOR UPDATE OF c`, conversationID, userID).Scan(&kind, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", model.Fail(404, "not_found")
	}
	if err != nil {
		return "", err
	}
	if kind != "group" {
		return "", model.Fail(400, "invalid_conversation")
	}
	return role, nil
}

// systemNotice appends one roamed system message to a conversation.
func (chat *Chat) systemNotice(ctx context.Context, tx pgx.Tx, conversationID, senderID, content string) error {
	var seq int64
	if err := tx.QueryRow(ctx, "UPDATE conversations SET next_seq=next_seq+1 WHERE id=$1 RETURNING next_seq", conversationID).Scan(&seq); err != nil {
		return err
	}
	message, err := insertMessage(ctx, tx, model.Message{ConversationID: conversationID, SenderID: senderID, ClientMsgID: "system:" + model.NewID(), Seq: seq, Content: content, Kind: "system"})
	if err != nil {
		return err
	}
	return chat.appendEvent(ctx, tx, conversationID, model.Event{Event: "message.created", Data: message})
}

// notifyConversation tells the remaining members that group metadata changed.
func (chat *Chat) notifyConversation(ctx context.Context, tx pgx.Tx, conversationID string) error {
	return chat.appendEvent(ctx, tx, conversationID, model.Event{Event: "conversation.updated", Data: map[string]string{"id": conversationID}})
}

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

// SetAnnouncement is gone: announcements are now individual rows managed by
// Announcements/CreateAnnouncement/UpdateAnnouncement/DeleteAnnouncement.
