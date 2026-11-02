package logic

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

// DiscoverGroups lists active groups visible for joining and the caller's request state.
func (chat *Chat) DiscoverGroups(ctx context.Context, callerID, query string) ([]model.DiscoverableGroup, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 100 {
		return nil, model.Fail(400, "invalid_query")
	}
	rows, err := chat.Store.DB.Query(ctx, `SELECT c.id,c.name,count(cm.user_id)::int,c.join_approval,
		COALESCE(jr.id,''),COALESCE(jr.status,'')
		FROM conversations c
		JOIN conversation_members cm ON cm.conversation_id=c.id
		LEFT JOIN conversation_join_requests jr ON jr.conversation_id=c.id AND jr.user_id=$1 AND jr.status='pending' AND jr.is_application
		WHERE c.kind='group' AND c.dismissed_at IS NULL
		AND ($2='' OR c.id=$2 OR c.name ILIKE '%'||$2||'%')
		AND NOT EXISTS (SELECT 1 FROM conversation_members mine WHERE mine.conversation_id=c.id AND mine.user_id=$1)
		GROUP BY c.id,c.name,c.join_approval,jr.id,jr.status ORDER BY lower(c.name),c.id LIMIT 100`, callerID, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []model.DiscoverableGroup{}
	for rows.Next() {
		var group model.DiscoverableGroup
		if err = rows.Scan(&group.ID, &group.Name, &group.MemberCount, &group.JoinApproval, &group.RequestID, &group.RequestStatus); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

// RequestToJoin adds the caller immediately when approval is disabled; otherwise
// it creates or idempotently refreshes a pending self-service application.
func (chat *Chat) RequestToJoin(ctx context.Context, callerID, conversationID, message string) (model.JoinRequest, error) {
	message = strings.TrimSpace(message)
	if len([]rune(message)) > 500 {
		return model.JoinRequest{}, model.Fail(400, "invalid_join_request")
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return model.JoinRequest{}, err
	}
	defer tx.Rollback(ctx)
	var kind string
	var approval, dismissed bool
	err = tx.QueryRow(ctx, `SELECT kind,join_approval,dismissed_at IS NOT NULL FROM conversations WHERE id=$1 FOR UPDATE`, conversationID).Scan(&kind, &approval, &dismissed)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.JoinRequest{}, model.Fail(404, "not_found")
	}
	if err != nil {
		return model.JoinRequest{}, err
	}
	if kind != "group" {
		return model.JoinRequest{}, model.Fail(400, "invalid_conversation")
	}
	if dismissed {
		return model.JoinRequest{}, model.Fail(409, "conversation_dismissed")
	}
	var member bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)", conversationID, callerID).Scan(&member); err != nil {
		return model.JoinRequest{}, err
	}
	if member {
		return model.JoinRequest{}, model.Fail(409, "already_member")
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM conversation_members WHERE conversation_id=$1", conversationID).Scan(&count); err != nil {
		return model.JoinRequest{}, err
	}
	if count >= MaxGroupMembers {
		return model.JoinRequest{}, model.Fail(400, "invalid_members")
	}
	if !approval {
		if _, err = tx.Exec(ctx, "INSERT INTO conversation_members(conversation_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING", conversationID, callerID); err != nil {
			return model.JoinRequest{}, err
		}
		if err = chat.appendEvent(ctx, tx, conversationID, model.Event{Event: "conversation.created", Data: map[string]string{"id": conversationID}}); err != nil {
			return model.JoinRequest{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE conversation_join_requests SET status='accepted',updated_at=now() WHERE conversation_id=$1 AND user_id=$2 AND status='pending'", conversationID, callerID); err != nil {
			return model.JoinRequest{}, err
		}
		if err = chat.systemNotice(ctx, tx, conversationID, callerID, "加入了群聊"); err != nil {
			return model.JoinRequest{}, err
		}
		if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
			return model.JoinRequest{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return model.JoinRequest{}, err
		}
		return model.JoinRequest{}, nil
	}
	var pending model.JoinRequest
	err = tx.QueryRow(ctx, `INSERT INTO conversation_join_requests(id,conversation_id,user_id,message,is_application)
		VALUES($1,$2,$3,$4,true) ON CONFLICT (conversation_id,user_id) WHERE status='pending' DO UPDATE SET message=EXCLUDED.message,updated_at=now(),is_application=true
		RETURNING id,conversation_id,user_id,message,status,created_at,updated_at,is_application`, model.NewID(), conversationID, callerID, message).
		Scan(&pending.ID, &pending.ConversationID, &pending.UserID, &pending.Message, &pending.Status, &pending.CreatedAt, &pending.UpdatedAt, &pending.IsApplication)
	if err != nil {
		return model.JoinRequest{}, err
	}
	var email, name, avatar string
	if err = tx.QueryRow(ctx, "SELECT email,name,COALESCE(avatar_media_id,'') FROM users WHERE id=$1", callerID).Scan(&email, &name, &avatar); err != nil {
		return model.JoinRequest{}, err
	}
	pending.User = model.User{ID: callerID, Email: email, Name: name, Avatar: avatar}
	if err = tx.Commit(ctx); err != nil {
		return model.JoinRequest{}, err
	}
	return pending, nil
}

// CancelJoinRequest withdraws only the caller's own pending application.
func (chat *Chat) CancelJoinRequest(ctx context.Context, callerID, conversationID string) error {
	command, err := chat.Store.DB.Exec(ctx, `DELETE FROM conversation_join_requests WHERE conversation_id=$1 AND user_id=$2 AND status='pending' AND is_application`, conversationID, callerID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return model.Fail(404, "not_found")
	}
	return nil
}

// MyJoinRequests returns application history and current group names.
func (chat *Chat) MyJoinRequests(ctx context.Context, callerID string) ([]model.JoinRequest, error) {
	rows, err := chat.Store.DB.Query(ctx, `SELECT jr.id,jr.conversation_id,jr.user_id,jr.message,jr.is_application,jr.status,jr.created_at,jr.updated_at,
		u.id,u.email,u.name,COALESCE(u.avatar_media_id,''),c.name
		FROM conversation_join_requests jr JOIN users u ON u.id=jr.user_id JOIN conversations c ON c.id=jr.conversation_id
		WHERE jr.user_id=$1 AND jr.is_application ORDER BY jr.updated_at DESC LIMIT 100`, callerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := []model.JoinRequest{}
	for rows.Next() {
		var request model.JoinRequest
		if err = rows.Scan(&request.ID, &request.ConversationID, &request.UserID, &request.Message, &request.IsApplication, &request.Status, &request.CreatedAt, &request.UpdatedAt, &request.User.ID, &request.User.Email, &request.User.Name, &request.User.Avatar, &request.GroupName); err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}
