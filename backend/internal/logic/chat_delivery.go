package logic

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
	"websocket_chat_go/backend/internal/model"
)

func (chat *Chat) DeliveryStatus(ctx context.Context, senderID, conversationID string, seq int64) (model.DeliveryStatus, error) {
	status := model.DeliveryStatus{ConversationID: conversationID, Seq: seq}
	if seq < 1 {
		return status, model.Fail(404, "not_found")
	}
	err := chat.Store.DB.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM conversation_members cm WHERE cm.conversation_id=m.conversation_id AND cm.user_id<>m.sender_id),
		(SELECT count(*) FROM conversation_members cm WHERE cm.conversation_id=m.conversation_id AND cm.user_id<>m.sender_id
			AND EXISTS (SELECT 1 FROM device_cursors dc WHERE dc.conversation_id=cm.conversation_id
				AND dc.user_id=cm.user_id AND dc.received_seq>=m.seq))
		FROM messages m WHERE m.conversation_id=$1 AND m.seq=$2 AND m.sender_id=$3`, conversationID, seq, senderID).Scan(&status.TotalMembers, &status.ConfirmedMembers)
	if errors.Is(err, pgx.ErrNoRows) {
		return status, model.Fail(404, "not_found")
	}
	status.AsOf = time.Now().UTC()
	return status, err
}
