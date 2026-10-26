package logic

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

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

// maxMemberAliasRunes bounds the group card a member can carry.
const maxMemberAliasRunes = 32

// maxMuteMinutes bounds a timed mute to thirty days.
const maxMuteMinutes = 30 * 24 * 60

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

// SetMemberMute silences one member for the given minutes, or lifts the mute
// when minutes is zero. An admin may only mute plain members; the owner may
// also mute admins, never another owner.
func (chat *Chat) SetMemberMute(ctx context.Context, callerID, conversationID, targetID string, minutes int) error {
	if targetID == "" || minutes < 0 || minutes > maxMuteMinutes {
		return model.Fail(400, "invalid_mute")
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
	if callerRole == model.RoleMember {
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
	if targetRole == model.RoleOwner || (callerRole == model.RoleAdmin && targetRole == model.RoleAdmin) {
		return model.Fail(403, "forbidden")
	}
	var mutedUntil any
	notice := ""
	if minutes > 0 {
		mutedUntil = time.Now().Add(time.Duration(minutes) * time.Minute)
		var targetName, callerName string
		if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", targetID).Scan(&targetName); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", callerID).Scan(&callerName); err != nil {
			return err
		}
		notice = callerName + "禁言了" + targetName + " " + strconv.Itoa(minutes) + " 分钟"
	} else {
		var targetName, callerName string
		if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", targetID).Scan(&targetName); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", callerID).Scan(&callerName); err != nil {
			return err
		}
		notice = callerName + "解除了对" + targetName + "的禁言"
	}
	if _, err = tx.Exec(ctx, "UPDATE conversation_members SET muted_until=$1 WHERE conversation_id=$2 AND user_id=$3", mutedUntil, conversationID, targetID); err != nil {
		return err
	}
	if err = chat.systemNotice(ctx, tx, conversationID, callerID, notice); err != nil {
		return err
	}
	if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Settings flips the group-wide toggles. Only admins and the owner may change
// them, and the response reports the values now in force.
func (chat *Chat) Settings(ctx context.Context, callerID, conversationID string, input model.ConversationSettingsInput) (model.ConversationSettings, error) {
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return model.ConversationSettings{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = chat.roleOf(ctx, tx, callerID, conversationID); err != nil {
		return model.ConversationSettings{}, err
	}
	if input.AllMuted != nil {
		if _, err = tx.Exec(ctx, "UPDATE conversations SET all_muted=$1 WHERE id=$2", *input.AllMuted, conversationID); err != nil {
			return model.ConversationSettings{}, err
		}
	}
	if input.JoinApproval != nil {
		if _, err = tx.Exec(ctx, "UPDATE conversations SET join_approval=$1 WHERE id=$2", *input.JoinApproval, conversationID); err != nil {
			return model.ConversationSettings{}, err
		}
	}
	var settings model.ConversationSettings
	if err = tx.QueryRow(ctx, "SELECT all_muted,join_approval FROM conversations WHERE id=$1", conversationID).Scan(&settings.AllMuted, &settings.JoinApproval); err != nil {
		return model.ConversationSettings{}, err
	}
	if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
		return model.ConversationSettings{}, err
	}
	return settings, tx.Commit(ctx)
}

// DismissGroup closes a group for good: only the owner may do it, history stays
// readable, and no further messages are accepted.
func (chat *Chat) DismissGroup(ctx context.Context, callerID, conversationID string) error {
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	callerRole, err := chat.roleOf(ctx, tx, callerID, conversationID)
	if err != nil {
		return err
	}
	if callerRole != model.RoleOwner {
		return model.Fail(403, "forbidden")
	}
	var dismissed bool
	if err = tx.QueryRow(ctx, "SELECT dismissed_at IS NOT NULL FROM conversations WHERE id=$1", conversationID).Scan(&dismissed); err != nil {
		return err
	}
	if dismissed {
		return model.Fail(409, "already_dismissed")
	}
	if _, err = tx.Exec(ctx, "UPDATE conversations SET dismissed_at=now() WHERE id=$1", conversationID); err != nil {
		return err
	}
	if err = chat.systemNotice(ctx, tx, conversationID, callerID, "群主解散了群聊"); err != nil {
		return err
	}
	if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AddMembers invites users into a group. With join approval enabled a plain
// member's invite becomes a pending request that an admin decides; admins and
// the owner still add directly.
func (chat *Chat) AddMembers(ctx context.Context, userID, conversationID string, input model.AddMembersInput) (model.AddMembersResult, error) {
	result := model.AddMembersResult{ConversationID: conversationID}
	if len(input.MemberIDs) < 1 || len(input.MemberIDs) >= MaxGroupMembers {
		return result, model.Fail(400, "invalid_members")
	}
	ids := append([]string(nil), input.MemberIDs...)
	slices.Sort(ids)
	for index, memberID := range ids {
		if memberID == "" || index > 0 && memberID == ids[index-1] {
			return result, model.Fail(400, "invalid_members")
		}
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	var kind, callerRole string
	var joinApproval bool
	if err = tx.QueryRow(ctx, `SELECT c.kind,c.join_approval,cm.role FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id WHERE c.id=$1 AND cm.user_id=$2 FOR UPDATE OF c`, conversationID, userID).Scan(&kind, &joinApproval, &callerRole); errors.Is(err, pgx.ErrNoRows) {
		return result, model.Fail(404, "not_found")
	} else if err != nil {
		return result, err
	}
	if kind != "group" {
		return result, model.Fail(400, "invalid_conversation")
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE id=ANY($1)", ids).Scan(&count); err != nil {
		return result, err
	}
	if count != len(ids) {
		return result, model.Fail(400, "invalid_members")
	}
	var existing int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM conversation_members WHERE conversation_id=$1 AND user_id=ANY($2)", conversationID, ids).Scan(&existing); err != nil {
		return result, err
	}
	var total int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM conversation_members WHERE conversation_id=$1", conversationID).Scan(&total); err != nil {
		return result, err
	}
	if total+len(ids)-existing > MaxGroupMembers {
		return result, model.Fail(400, "invalid_members")
	}
	var inviterName string
	if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", userID).Scan(&inviterName); err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, "SELECT u.id,u.name FROM users u WHERE u.id=ANY($1) AND NOT EXISTS (SELECT 1 FROM conversation_members cm WHERE cm.conversation_id=$2 AND cm.user_id=u.id) ORDER BY u.id", ids, conversationID)
	if err != nil {
		return result, err
	}
	type invitedMember struct{ id, name string }
	invited := []invitedMember{}
	for rows.Next() {
		var member invitedMember
		if err = rows.Scan(&member.id, &member.name); err != nil {
			break
		}
		invited = append(invited, member)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(invited) == 0 {
		return result, tx.Commit(ctx)
	}
	// With join approval on, a plain member's invite waits for an admin. The
	// request is invisible to the invitee until it is accepted, and an existing
	// pending request is left untouched instead of duplicated.
	if joinApproval && callerRole == model.RoleMember {
		for _, member := range invited {
			if _, err = tx.Exec(ctx, `INSERT INTO conversation_join_requests(id,conversation_id,user_id,message)
				VALUES($1,$2,$3,$4) ON CONFLICT (conversation_id,user_id) WHERE status='pending' DO NOTHING`,
				model.NewID(), conversationID, member.id, "来自 "+inviterName+" 的邀请"); err != nil {
				return result, err
			}
		}
		result.Pending = true
		return result, tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, "INSERT INTO conversation_members(conversation_id,user_id) SELECT $1,unnest($2::text[]) ON CONFLICT DO NOTHING", conversationID, ids); err != nil {
		return result, err
	}
	if err = chat.appendEvent(ctx, tx, conversationID, model.Event{Event: "conversation.created", Data: map[string]string{"id": conversationID}}); err != nil {
		return result, err
	}
	for _, member := range invited {
		var seq int64
		if err = tx.QueryRow(ctx, "UPDATE conversations SET next_seq=next_seq+1 WHERE id=$1 RETURNING next_seq", conversationID).Scan(&seq); err != nil {
			return result, err
		}
		content := inviterName + "邀请" + member.name + "进入群聊"
		message, insertErr := insertMessage(ctx, tx, model.Message{ConversationID: conversationID, SenderID: userID, ClientMsgID: "system:" + model.NewID(), Seq: seq, Content: content, Kind: "system"})
		if insertErr != nil {
			return result, insertErr
		}
		if err = chat.appendEvent(ctx, tx, conversationID, model.Event{Event: "message.created", Data: message}); err != nil {
			return result, err
		}
	}
	return result, tx.Commit(ctx)
}

// DiscoverGroups lists active groups visible for joining and the caller's request state.
func (chat *Chat) DiscoverGroups(ctx context.Context, callerID, query string) ([]model.DiscoverableGroup, error) {
	if len([]rune(query)) > 100 {
		return nil, model.Fail(400, "invalid_query")
	}
	rows, err := chat.Store.DB.Query(ctx, `SELECT c.id,c.name,count(cm.user_id)::int,
		COALESCE(jr.id,''),COALESCE(jr.status,'')
		FROM conversations c
		JOIN conversation_members cm ON cm.conversation_id=c.id
		LEFT JOIN conversation_join_requests jr ON jr.conversation_id=c.id AND jr.user_id=$1 AND jr.status='pending'
		WHERE c.kind='group' AND c.dismissed_at IS NULL AND c.name ILIKE '%'||$2||'%'
		AND NOT EXISTS (SELECT 1 FROM conversation_members mine WHERE mine.conversation_id=c.id AND mine.user_id=$1)
		GROUP BY c.id,c.name,jr.id,jr.status ORDER BY lower(c.name),c.id LIMIT 100`, callerID, strings.TrimSpace(query))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []model.DiscoverableGroup{}
	for rows.Next() {
		var group model.DiscoverableGroup
		if err = rows.Scan(&group.ID, &group.Name, &group.MemberCount, &group.RequestID, &group.RequestStatus); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

// RequestToJoin asks the group owner or an admin to approve membership.
func (chat *Chat) RequestToJoin(ctx context.Context, callerID, conversationID, message string) (model.JoinRequest, error) {
	message = strings.TrimSpace(message)
	if len([]rune(message)) > 500 {
		return model.JoinRequest{}, model.Fail(400, "invalid_join_request")
	}
	storedMessage := "加入申请: " + message
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil { return model.JoinRequest{}, err }
	defer tx.Rollback(ctx)
	var kind string
	var approval bool
	err = tx.QueryRow(ctx, `SELECT kind,join_approval FROM conversations WHERE id=$1 AND dismissed_at IS NULL FOR UPDATE`, conversationID).Scan(&kind, &approval)
	if errors.Is(err, pgx.ErrNoRows) { return model.JoinRequest{}, model.Fail(404, "not_found") }
	if err != nil { return model.JoinRequest{}, err }
	if kind != "group" { return model.JoinRequest{}, model.Fail(400, "invalid_conversation") }
	if !approval { return model.JoinRequest{}, model.Fail(409, "join_requests_disabled") }
	var member bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)", conversationID, callerID).Scan(&member); err != nil { return model.JoinRequest{}, err }
	if member { return model.JoinRequest{}, model.Fail(409, "already_member") }
	var pending model.JoinRequest
	err = tx.QueryRow(ctx, `INSERT INTO conversation_join_requests(id,conversation_id,user_id,message)
		VALUES($1,$2,$3,$4) ON CONFLICT (conversation_id,user_id) WHERE status='pending' DO UPDATE SET message=EXCLUDED.message,updated_at=now()
		RETURNING id,conversation_id,user_id,message,status,created_at,updated_at`, model.NewID(), conversationID, callerID, storedMessage).Scan(&pending.ID, &pending.ConversationID, &pending.UserID, &pending.Message, &pending.Status, &pending.CreatedAt, &pending.UpdatedAt)
	if err != nil { return model.JoinRequest{}, err }
	var email, name, avatar string
	if err = tx.QueryRow(ctx, "SELECT email,name,COALESCE(avatar_media_id,'') FROM users WHERE id=$1", callerID).Scan(&email,&name,&avatar); err != nil { return model.JoinRequest{}, err }
	pending.User = model.User{ID: callerID, Email: email, Name: name, Avatar: avatar}
	if err = tx.Commit(ctx); err != nil { return model.JoinRequest{}, err }
	return pending, nil
}

// CancelJoinRequest withdraws the caller's pending application.
func (chat *Chat) CancelJoinRequest(ctx context.Context, callerID, conversationID string) error {
	command, err := chat.Store.DB.Exec(ctx, `DELETE FROM conversation_join_requests WHERE conversation_id=$1 AND user_id=$2 AND status='pending'`, conversationID, callerID)
	if err != nil { return err }
	if command.RowsAffected() == 0 { return model.Fail(404, "not_found") }
	return nil
}

// MyJoinRequests returns requests and decisions visible to their applicant.
func (chat *Chat) MyJoinRequests(ctx context.Context, callerID string) ([]model.JoinRequest, error) {
	rows, err := chat.Store.DB.Query(ctx, `SELECT jr.id,jr.conversation_id,jr.user_id,jr.message,jr.status,jr.created_at,jr.updated_at,
		u.id,u.email,u.name,COALESCE(u.avatar_media_id,''),c.name
		FROM conversation_join_requests jr JOIN users u ON u.id=jr.user_id JOIN conversations c ON c.id=jr.conversation_id
		WHERE jr.user_id=$1 AND jr.message LIKE '加入申请:%' ORDER BY jr.updated_at DESC LIMIT 100`, callerID)
	if err != nil { return nil, err }
	defer rows.Close()
	requests := []model.JoinRequest{}
	for rows.Next() {
		var request model.JoinRequest
		if err = rows.Scan(&request.ID,&request.ConversationID,&request.UserID,&request.Message,&request.Status,&request.CreatedAt,&request.UpdatedAt,&request.User.ID,&request.User.Email,&request.User.Name,&request.User.Avatar,&request.GroupName); err != nil { return nil, err }
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

// JoinRequests lists every pending application or invite for group administrators.
func (chat *Chat) JoinRequests(ctx context.Context, callerID, conversationID string) ([]model.JoinRequest, error) {
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	callerRole, err := chat.roleOf(ctx, tx, callerID, conversationID)
	if err != nil {
		return nil, err
	}
	if callerRole != model.RoleOwner && callerRole != model.RoleAdmin {
		return nil, model.Fail(403, "forbidden")
	}
	rows, err := tx.Query(ctx, `SELECT jr.id,jr.conversation_id,jr.user_id,jr.message,jr.status,jr.created_at,jr.updated_at,u.id,u.email,u.name,COALESCE(u.avatar_media_id,'')
		FROM conversation_join_requests jr JOIN users u ON u.id=jr.user_id
		WHERE jr.conversation_id=$1 AND jr.status='pending' ORDER BY jr.created_at,jr.id`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := []model.JoinRequest{}
	for rows.Next() {
		var request model.JoinRequest
		if err = rows.Scan(&request.ID, &request.ConversationID, &request.UserID, &request.Message, &request.Status, &request.CreatedAt, &request.UpdatedAt, &request.User.ID, &request.User.Email, &request.User.Name, &request.User.Avatar); err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

// DecideJoinRequest accepts or rejects one pending invite. Accepting adds the
// user to the group the same way a direct invite would have.
func (chat *Chat) DecideJoinRequest(ctx context.Context, callerID, conversationID, requestID, action string) (model.JoinRequest, bool, error) {
	accepted := action == "accept"
	if !accepted && action != "reject" {
		return model.JoinRequest{}, false, model.Fail(400, "invalid_join_request_action")
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return model.JoinRequest{}, false, err
	}
	defer tx.Rollback(ctx)
	callerRole, err := chat.roleOf(ctx, tx, callerID, conversationID)
	if err != nil {
		return model.JoinRequest{}, false, err
	}
	if callerRole != model.RoleOwner && callerRole != model.RoleAdmin {
		return model.JoinRequest{}, false, model.Fail(403, "forbidden")
	}
	var targetID string
	err = tx.QueryRow(ctx, `SELECT user_id FROM conversation_join_requests WHERE id=$1 AND conversation_id=$2 AND status='pending' FOR UPDATE`, requestID, conversationID).Scan(&targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.JoinRequest{}, false, model.Fail(404, "not_found")
	}
	if err != nil {
		return model.JoinRequest{}, false, err
	}
	status := "rejected"
	if accepted {
		status = "accepted"
	}
	if _, err = tx.Exec(ctx, "UPDATE conversation_join_requests SET status=$1,updated_at=now() WHERE id=$2", status, requestID); err != nil {
		return model.JoinRequest{}, false, err
	}
	request, err := scanJoinRequest(tx.QueryRow(ctx, `SELECT jr.id,jr.conversation_id,jr.user_id,jr.message,jr.status,jr.created_at,jr.updated_at,u.id,u.email,u.name,COALESCE(u.avatar_media_id,'')
		FROM conversation_join_requests jr JOIN users u ON u.id=jr.user_id WHERE jr.id=$1`, requestID))
	if err != nil {
		return model.JoinRequest{}, false, err
	}
	if accepted {
		var count int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM conversation_members WHERE conversation_id=$1", conversationID).Scan(&count); err != nil {
			return model.JoinRequest{}, false, err
		}
		if count >= MaxGroupMembers {
			return model.JoinRequest{}, false, model.Fail(400, "invalid_members")
		}
		if _, err = tx.Exec(ctx, "INSERT INTO conversation_members(conversation_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING", conversationID, targetID); err != nil {
			return model.JoinRequest{}, false, err
		}
		if err = chat.appendEvent(ctx, tx, conversationID, model.Event{Event: "conversation.created", Data: map[string]string{"id": conversationID}}); err != nil {
			return model.JoinRequest{}, false, err
		}
		var targetName string
		if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", targetID).Scan(&targetName); err != nil {
			return model.JoinRequest{}, false, err
		}
		if err = chat.systemNotice(ctx, tx, conversationID, callerID, "同意了"+targetName+"加入群聊"); err != nil {
			return model.JoinRequest{}, false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return model.JoinRequest{}, false, err
	}
	return request, accepted, nil
}

func scanJoinRequest(row pgx.Row) (model.JoinRequest, error) {
	var request model.JoinRequest
	err := row.Scan(&request.ID, &request.ConversationID, &request.UserID, &request.Message, &request.Status, &request.CreatedAt, &request.UpdatedAt, &request.User.ID, &request.User.Email, &request.User.Name, &request.User.Avatar)
	return request, err
}
