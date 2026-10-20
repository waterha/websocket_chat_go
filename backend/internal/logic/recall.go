package logic

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

// recallWindow bounds how long after sending a message may be recalled. Beyond
// it the message is treated as permanent history, which keeps a shared record
// trustworthy instead of letting anyone rewrite the past indefinitely.
const recallWindow = 2 * time.Minute

// Recall lets the sender withdraw one of their own messages. Group owners and
// admins may also recall any message in their group, which is the moderation
// case. A recalled message keeps its row and sequence so history stays
// contiguous; its content and attachment become unavailable and clients render a
// placeholder. The operation is idempotent.
func (chat *Chat) Recall(ctx context.Context, userID, conversationID string, seq int64) (model.Message, error) {
	if seq < 1 {
		return model.Message{}, model.Fail(404, "not_found")
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return model.Message{}, err
	}
	defer tx.Rollback(ctx)
	var kind string
	var callerRole string
	err = tx.QueryRow(ctx, `SELECT c.kind,cm.role FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id
		WHERE c.id=$1 AND cm.user_id=$2 FOR UPDATE OF c`, conversationID, userID).Scan(&kind, &callerRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Message{}, model.Fail(404, "not_found")
	}
	if err != nil {
		return model.Message{}, err
	}
	var senderID string
	var recalledAt *time.Time
	var createdAt time.Time
	err = tx.QueryRow(ctx, "SELECT sender_id,recalled_at,created_at FROM messages WHERE conversation_id=$1 AND seq=$2 FOR UPDATE", conversationID, seq).Scan(&senderID, &recalledAt, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Message{}, model.Fail(404, "not_found")
	}
	if err != nil {
		return model.Message{}, err
	}
	if recalledAt != nil {
		// Already recalled: return the current state without emitting again.
		return scanMessage(tx.QueryRow(ctx, "SELECT "+messageColumns+" FROM messages WHERE conversation_id=$1 AND seq=$2", conversationID, seq))
	}
	allowed := senderID == userID
	if !allowed && kind == "group" && (callerRole == model.RoleOwner || callerRole == model.RoleAdmin) {
		allowed = true
	}
	if !allowed {
		return model.Message{}, model.Fail(403, "forbidden")
	}
	// Only the original sender is bound by the time window; moderators are not.
	if senderID == userID && time.Since(createdAt) > recallWindow {
		return model.Message{}, model.Fail(403, "recall_window_expired")
	}
	if _, err = tx.Exec(ctx, "UPDATE messages SET recalled_at=now() WHERE conversation_id=$1 AND seq=$2", conversationID, seq); err != nil {
		return model.Message{}, err
	}
	message, err := scanMessage(tx.QueryRow(ctx, "SELECT "+messageColumns+" FROM messages WHERE conversation_id=$1 AND seq=$2", conversationID, seq))
	if err != nil {
		return model.Message{}, err
	}
	if err = chat.appendEvent(ctx, tx, conversationID, model.Event{Event: "message.recalled", Data: message}); err != nil {
		return model.Message{}, err
	}
	return message, tx.Commit(ctx)
}
