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
	validContent := len([]rune(input.Content)) >= 1 && len([]rune(input.Content)) <= 4000
	// A media message may carry an optional caption but must reference an
	// upload, so an empty caption is only acceptable together with media_id.
	if input.ConversationID == "" || len(input.ClientMsgID) < 8 || len(input.ClientMsgID) > 100 || (input.MediaID == "" && !validContent) || (input.MediaID != "" && len([]rune(input.Content)) > 4000) {
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
	kind := "user"
	if input.MediaID != "" {
		// An attachment must belong to the sender and to this conversation.
		var mediaKind string
		err = tx.QueryRow(ctx, "SELECT media_type FROM message_media WHERE id=$1 AND owner_id=$2 AND conversation_id=$3", input.MediaID, userID, input.ConversationID).Scan(&mediaKind)
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Message{}, false, model.Fail(404, "not_found")
		}
		if err != nil {
			return model.Message{}, false, err
		}
		kind = mediaKind
	}
	existing, err := scanMessage(tx.QueryRow(ctx, "SELECT "+messageColumns+" FROM messages WHERE sender_id=$1 AND client_msg_id=$2", userID, input.ClientMsgID))
	if err == nil {
		if existing.ConversationID != input.ConversationID || existing.Content != input.Content || existing.MediaID != input.MediaID {
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
	message, err := insertMessage(ctx, tx, model.Message{ConversationID: input.ConversationID, SenderID: userID, ClientMsgID: input.ClientMsgID, Seq: seq, Content: input.Content, Kind: kind, MediaID: input.MediaID})
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
