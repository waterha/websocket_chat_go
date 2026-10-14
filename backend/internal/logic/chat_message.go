package logic

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
	"websocket_chat_go/backend/internal/model"
)

func ValidateSend(input model.SendInput) (model.SendInput, error) {
	input.Content = strings.TrimSpace(input.Content)
	if input.ConversationID == "" || len(input.ClientMsgID) < 8 || len(input.ClientMsgID) > 100 || len([]rune(input.Content)) < 1 || len([]rune(input.Content)) > 4000 {
		return input, model.Fail(400, "invalid_message")
	}
	return input, nil
}

func (chat *Chat) Send(ctx context.Context, userID string, input model.SendInput) (model.Message, bool, error) {
	input, err := ValidateSend(input)
	if err != nil {
		return model.Message{}, false, err
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return model.Message{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", "send:"+userID); err != nil {
		return model.Message{}, false, err
	}
	var member bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)", input.ConversationID, userID).Scan(&member); err != nil {
		return model.Message{}, false, err
	}
	if !member {
		return model.Message{}, false, model.Fail(404, "not_found")
	}
	existing, err := scanMessage(tx.QueryRow(ctx, "SELECT "+messageColumns+" FROM messages WHERE sender_id=$1 AND client_msg_id=$2", userID, input.ClientMsgID))
	if err == nil {
		if existing.ConversationID != input.ConversationID || existing.Content != input.Content {
			return model.Message{}, false, model.Fail(409, "message_conflict")
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Message{}, false, err
	}
	var seq int64
	if err = tx.QueryRow(ctx, "UPDATE conversations SET next_seq=next_seq+1 WHERE id=$1 RETURNING next_seq", input.ConversationID).Scan(&seq); err != nil {
		return model.Message{}, false, err
	}
	message, err := scanMessage(tx.QueryRow(ctx, `INSERT INTO messages(id,conversation_id,sender_id,client_msg_id,seq,content) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+messageColumns, model.NewID(), input.ConversationID, userID, input.ClientMsgID, seq, input.Content))
	if err != nil {
		return model.Message{}, false, err
	}
	if err = chat.appendEvent(ctx, tx, input.ConversationID, model.Event{Event: "message.created", Data: message}); err != nil {
		return model.Message{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return model.Message{}, false, err
	}
	return message, false, nil
}
