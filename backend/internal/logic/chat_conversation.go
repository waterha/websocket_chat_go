package logic

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
	"websocket_chat_go/backend/internal/model"
)

func (chat *Chat) ConversationSeq(ctx context.Context, userID, conversationID string) (int64, error) {
	var seq int64
	err := chat.Store.DB.QueryRow(ctx, `SELECT c.next_seq FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id WHERE c.id=$1 AND cm.user_id=$2`, conversationID, userID).Scan(&seq)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, model.Fail(404, "not_found")
	}
	return seq, err
}

func (chat *Chat) Conversations(ctx context.Context, userID string) ([]model.Conversation, error) {
	rows, err := chat.Store.DB.Query(ctx, `SELECT c.id,c.name,c.kind,c.announcement,COALESCE(c.created_by,''),c.next_seq,cm.last_read_seq,cm.pinned_at IS NOT NULL,cm.muted,cm.archived_at IS NOT NULL,
		(SELECT jsonb_agg(jsonb_build_object('id',u.id,'name',u.name,'email',u.email,'role',members.role,'last_read_seq',members.last_read_seq,'read_at',members.read_at) ORDER BY (members.role='owner') DESC,(members.role='admin') DESC,u.name,u.id) FROM conversation_members members JOIN users u ON u.id=members.user_id WHERE members.conversation_id=c.id)
		FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id WHERE cm.user_id=$1
		ORDER BY (cm.pinned_at IS NULL),cm.pinned_at DESC,c.created_at DESC,c.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	conversations := []model.Conversation{}
	for rows.Next() {
		var item model.Conversation
		var members []byte
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.Announcement, &item.OwnerID, &item.LastSeq, &item.LastReadSeq, &item.Pinned, &item.Muted, &item.Archived, &members); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(members, &item.Members); err != nil {
			return nil, err
		}
		conversations = append(conversations, item)
	}
	return conversations, rows.Err()
}

func (chat *Chat) Create(ctx context.Context, userID string, input model.CreateInput) (string, error) {
	input.Name = strings.TrimSpace(input.Name)
	if (input.Kind != "direct" && input.Kind != "group") || (input.Kind == "direct" && len(input.MemberIDs) != 1) || (input.Kind == "group" && (len(input.MemberIDs) >= MaxGroupMembers || len([]rune(input.Name)) < 1 || len([]rune(input.Name)) > 100)) {
		return "", model.Fail(400, "invalid_conversation")
	}
	ids := append([]string{userID}, input.MemberIDs...)
	slices.Sort(ids)
	for index, memberID := range ids {
		if memberID == "" || index > 0 && memberID == ids[index-1] {
			return "", model.Fail(400, "invalid_members")
		}
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE id=ANY($1)", ids).Scan(&count); err != nil {
		return "", err
	}
	if count != len(ids) {
		return "", model.Fail(400, "invalid_members")
	}
	conversationID := model.NewID()
	// The creator owns a new group; direct conversations keep no owner.
	ownerID := any(nil)
	if input.Kind == "group" {
		ownerID = userID
	}
	var directKey any
	if input.Kind == "direct" {
		directKey = strings.Join(ids, ":")
		input.Name = ""
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "direct:"+directKey.(string)); err != nil {
			return "", err
		}
		var existing string
		err = tx.QueryRow(ctx, "SELECT id FROM conversations WHERE direct_key=$1", directKey).Scan(&existing)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO conversations(id,name,kind,direct_key,created_by) VALUES($1,$2,$3,$4,$5)", conversationID, input.Name, input.Kind, directKey, ownerID); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO conversation_members(conversation_id,user_id,role) SELECT $1,id,CASE WHEN id=$3 AND $4='group' THEN 'owner' ELSE 'member' END FROM unnest($2::text[]) AS id", conversationID, ids, userID, input.Kind); err != nil {
		return "", err
	}
	if err = chat.appendEvent(ctx, tx, conversationID, model.Event{Event: "conversation.created", Data: map[string]string{"id": conversationID}}); err != nil {
		return "", err
	}
	return conversationID, tx.Commit(ctx)
}

func (chat *Chat) AddMembers(ctx context.Context, userID, conversationID string, input model.AddMembersInput) error {
	if len(input.MemberIDs) < 1 || len(input.MemberIDs) >= MaxGroupMembers {
		return model.Fail(400, "invalid_members")
	}
	ids := append([]string(nil), input.MemberIDs...)
	slices.Sort(ids)
	for index, memberID := range ids {
		if memberID == "" || index > 0 && memberID == ids[index-1] {
			return model.Fail(400, "invalid_members")
		}
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var kind string
	if err = tx.QueryRow(ctx, `SELECT c.kind FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id WHERE c.id=$1 AND cm.user_id=$2 FOR UPDATE OF c`, conversationID, userID).Scan(&kind); errors.Is(err, pgx.ErrNoRows) {
		return model.Fail(404, "not_found")
	} else if err != nil {
		return err
	}
	if kind != "group" {
		return model.Fail(400, "invalid_conversation")
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE id=ANY($1)", ids).Scan(&count); err != nil {
		return err
	}
	if count != len(ids) {
		return model.Fail(400, "invalid_members")
	}
	var existing int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM conversation_members WHERE conversation_id=$1 AND user_id=ANY($2)", conversationID, ids).Scan(&existing); err != nil {
		return err
	}
	var total int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM conversation_members WHERE conversation_id=$1", conversationID).Scan(&total); err != nil {
		return err
	}
	if total+len(ids)-existing > MaxGroupMembers {
		return model.Fail(400, "invalid_members")
	}
	var inviterName string
	if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", userID).Scan(&inviterName); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT u.id,u.name FROM users u WHERE u.id=ANY($1) AND NOT EXISTS (SELECT 1 FROM conversation_members cm WHERE cm.conversation_id=$2 AND cm.user_id=u.id) ORDER BY u.id", ids, conversationID)
	if err != nil {
		return err
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
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO conversation_members(conversation_id,user_id) SELECT $1,unnest($2::text[]) ON CONFLICT DO NOTHING", conversationID, ids); err != nil {
		return err
	}
	if len(invited) > 0 {
		if err = chat.appendEvent(ctx, tx, conversationID, model.Event{Event: "conversation.created", Data: map[string]string{"id": conversationID}}); err != nil {
			return err
		}
		for _, member := range invited {
			var seq int64
			if err = tx.QueryRow(ctx, "UPDATE conversations SET next_seq=next_seq+1 WHERE id=$1 RETURNING next_seq", conversationID).Scan(&seq); err != nil {
				return err
			}
			content := inviterName + "邀请" + member.name + "进入群聊"
			message, insertErr := insertMessage(ctx, tx, model.Message{ConversationID: conversationID, SenderID: userID, ClientMsgID: "system:" + model.NewID(), Seq: seq, Content: content, Kind: "system"})
			if insertErr != nil {
				return insertErr
			}
			if err = chat.appendEvent(ctx, tx, conversationID, model.Event{Event: "message.created", Data: message}); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}
