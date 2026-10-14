package logic

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

type Chat struct{ Store *model.Store }

const MaxGroupMembers = 1000

const messageColumns = "id,conversation_id,sender_id,client_msg_id,seq,content,created_at"

func scanMessage(row pgx.Row) (model.Message, error) {
	var message model.Message
	err := row.Scan(&message.ID, &message.ConversationID, &message.SenderID, &message.ClientMsgID, &message.Seq, &message.Content, &message.CreatedAt)
	return message, err
}

func (chat *Chat) appendEvent(ctx context.Context, tx pgx.Tx, conversationID string, event model.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO message_outbox(conversation_id,recipients,payload)
		SELECT $1,array_agg(user_id ORDER BY user_id),$2::jsonb FROM conversation_members WHERE conversation_id=$1`, conversationID, payload)
	return err
}
