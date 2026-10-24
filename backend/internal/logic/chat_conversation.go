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
	rows, err := chat.Store.DB.Query(ctx, `SELECT c.id,c.name,c.kind,COALESCE(c.created_by,''),c.next_seq,cm.last_read_seq,cm.pinned_at IS NOT NULL,cm.muted,cm.alias,cm.deleted_at,
		c.all_muted,c.join_approval,c.dismissed_at IS NOT NULL,
		(SELECT count(*) FROM conversation_join_requests jr WHERE jr.conversation_id=c.id AND jr.status='pending'),
		(SELECT jsonb_agg(jsonb_build_object('id',u.id,'name',u.name,'email',u.email,'role',members.role,'last_read_seq',members.last_read_seq,'read_at',members.read_at,'alias',members.alias,'muted_until',members.muted_until) ORDER BY (members.role='owner') DESC,(members.role='admin') DESC,u.name,u.id) FROM conversation_members members JOIN users u ON u.id=members.user_id WHERE members.conversation_id=c.id),
		(SELECT count(*) FROM announcements a WHERE a.conversation_id=c.id)
		FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id WHERE cm.user_id=$1 AND cm.deleted_at IS NULL
		ORDER BY (cm.pinned_at IS NULL),cm.pinned_at DESC,c.created_at DESC,c.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	conversations := []model.Conversation{}
	for rows.Next() {
		var item model.Conversation
		var members []byte
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.OwnerID, &item.LastSeq, &item.LastReadSeq, &item.Pinned, &item.Muted, &item.Alias, &item.DeletedAt, &item.AllMuted, &item.JoinApproval, &item.Dismissed, &item.PendingJoins, &members, &item.AnnouncementCount); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(members, &item.Members); err != nil {
			return nil, err
		}
		conversations = append(conversations, item)
	}
	return conversations, rows.Err()
}

// DeleteConversation hides one conversation from the caller's sidebar. The
// membership stays, so the next incoming message clears the marker and the
// conversation reappears with its history intact.
func (chat *Chat) DeleteConversation(ctx context.Context, userID, conversationID string) error {
	tag, err := chat.Store.DB.Exec(ctx, `UPDATE conversation_members SET deleted_at=now(),pinned_at=NULL,muted=false
		WHERE conversation_id=$1 AND user_id=$2`, conversationID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.Fail(404, "not_found")
	}
	return nil
}

// MemberIDs lists the members of a conversation the caller belongs to. It backs
// ephemeral relays such as typing, which never touch the outbox. A non-member
// learns nothing about the room, not even an empty list.
func (chat *Chat) MemberIDs(ctx context.Context, userID, conversationID string) ([]string, error) {
	var member bool
	if err := chat.Store.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)", conversationID, userID).Scan(&member); err != nil {
		return nil, err
	}
	if !member {
		return nil, model.Fail(404, "not_found")
	}
	rows, err := chat.Store.DB.Query(ctx, "SELECT user_id FROM conversation_members WHERE conversation_id=$1", conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		members = append(members, id)
	}
	return members, rows.Err()
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
		// A direct conversation needs both sides willing: either block stops the
		// conversation from being (re)opened here.
		if blocked, err := blockedBetween(ctx, tx, input.MemberIDs[0], userID); err != nil {
			return "", err
		} else if blocked {
			return "", model.Fail(403, "blocked_by_user")
		}
		if blocked, err := blockedBetween(ctx, tx, userID, input.MemberIDs[0]); err != nil {
			return "", err
		} else if blocked {
			return "", model.Fail(400, "user_blocked")
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
