package logic

import (
	"context"

	"websocket_chat_go/backend/internal/model"
)

// Poke adds a playful "拍一拍" system notice aimed at one member. Both sides
// must belong to the conversation, and the notice roams like every other
// system message.
func (chat *Chat) Poke(ctx context.Context, userID, conversationID, targetID string) error {
	if targetID == "" {
		return model.Fail(400, "invalid_members")
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM conversation_members WHERE conversation_id=$1 AND user_id=ANY($2)`, conversationID, []string{userID, targetID}).Scan(&count); err != nil {
		return err
	}
	if count != 2 {
		return model.Fail(404, "not_found")
	}
	var callerName, targetName string
	if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", userID).Scan(&callerName); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", targetID).Scan(&targetName); err != nil {
		return err
	}
	if err = chat.systemNotice(ctx, tx, conversationID, userID, "「"+callerName+"」拍了拍「"+targetName+"」"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
