package logic

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

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
	var joinApproval, dismissed bool
	if err = tx.QueryRow(ctx, `SELECT c.kind,c.join_approval,cm.role,c.dismissed_at IS NOT NULL FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id WHERE c.id=$1 AND cm.user_id=$2 FOR UPDATE OF c`, conversationID, userID).Scan(&kind, &joinApproval, &callerRole, &dismissed); errors.Is(err, pgx.ErrNoRows) {
		return result, model.Fail(404, "not_found")
	} else if err != nil {
		return result, err
	}
	if kind != "group" {
		return result, model.Fail(400, "invalid_conversation")
	}
	if dismissed {
		return result, model.Fail(409, "conversation_dismissed")
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
