package logic

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

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
	rows, err := tx.Query(ctx, `SELECT jr.id,jr.conversation_id,jr.user_id,jr.message,jr.is_application,jr.status,jr.created_at,jr.updated_at,u.id,u.email,u.name,COALESCE(u.avatar_media_id,''),c.name
		FROM conversation_join_requests jr JOIN users u ON u.id=jr.user_id JOIN conversations c ON c.id=jr.conversation_id
		WHERE jr.conversation_id=$1 AND jr.status='pending' ORDER BY jr.created_at,jr.id`, conversationID)
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

// DecideJoinRequest accepts or rejects one pending invite or application.
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
	var kind string
	var dismissed bool
	if err = tx.QueryRow(ctx, "SELECT kind,dismissed_at IS NOT NULL FROM conversations WHERE id=$1 FOR UPDATE", conversationID).Scan(&kind, &dismissed); err != nil {
		return model.JoinRequest{}, false, err
	}
	if kind != "group" {
		return model.JoinRequest{}, false, model.Fail(400, "invalid_conversation")
	}
	if dismissed && accepted {
		return model.JoinRequest{}, false, model.Fail(409, "conversation_dismissed")
	}
	status := "rejected"
	if accepted {
		status = "accepted"
	}
	if _, err = tx.Exec(ctx, "UPDATE conversation_join_requests SET status=$1,updated_at=now() WHERE id=$2", status, requestID); err != nil {
		return model.JoinRequest{}, false, err
	}
	request, err := scanJoinRequest(tx.QueryRow(ctx, `SELECT jr.id,jr.conversation_id,jr.user_id,jr.message,jr.status,jr.created_at,jr.updated_at,u.id,u.email,u.name,COALESCE(u.avatar_media_id,''),jr.is_application,c.name
		FROM conversation_join_requests jr JOIN users u ON u.id=jr.user_id JOIN conversations c ON c.id=jr.conversation_id WHERE jr.id=$1`, requestID))
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
	err := row.Scan(&request.ID, &request.ConversationID, &request.UserID, &request.Message, &request.Status, &request.CreatedAt, &request.UpdatedAt, &request.User.ID, &request.User.Email, &request.User.Name, &request.User.Avatar, &request.IsApplication, &request.GroupName)
	return request, err
}
