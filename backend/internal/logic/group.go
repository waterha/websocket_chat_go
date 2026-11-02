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
