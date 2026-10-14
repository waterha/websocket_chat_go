package logic

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

func (chat *Chat) MarkRead(ctx context.Context, userID, conversationID string, seq int64) (model.Receipt, error) {
	receipt := model.Receipt{ConversationID: conversationID, UserID: userID}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return receipt, err
	}
	defer tx.Rollback(ctx)
	var lastSeq int64
	err = tx.QueryRow(ctx, `SELECT next_seq FROM conversations WHERE id=$1 AND EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2) FOR UPDATE`, conversationID, userID).Scan(&lastSeq)
	if errors.Is(err, pgx.ErrNoRows) {
		return receipt, model.Fail(404, "not_found")
	}
	if err != nil {
		return receipt, err
	}
	if seq < 0 || seq > lastSeq {
		return receipt, model.Fail(400, "invalid_seq")
	}
	var previous int64
	if err = tx.QueryRow(ctx, "SELECT last_read_seq FROM conversation_members WHERE conversation_id=$1 AND user_id=$2", conversationID, userID).Scan(&previous); err != nil {
		return receipt, err
	}
	err = tx.QueryRow(ctx, `UPDATE conversation_members SET last_read_seq=GREATEST(last_read_seq,$3), read_at=CASE WHEN $3>last_read_seq THEN now() ELSE read_at END WHERE conversation_id=$1 AND user_id=$2 RETURNING last_read_seq,read_at`, conversationID, userID, seq).Scan(&receipt.Seq, &receipt.ReadAt)
	if err != nil {
		return receipt, err
	}
	if seq > previous {
		if err = chat.appendEvent(ctx, tx, conversationID, model.Event{Event: "conversation.read", Data: receipt}); err != nil {
			return receipt, err
		}
	}
	return receipt, tx.Commit(ctx)
}

func (chat *Chat) MarkReceived(ctx context.Context, userID, conversationID, deviceID string, seq int64) (model.DeviceCursor, error) {
	cursor := model.DeviceCursor{ConversationID: conversationID, DeviceID: deviceID}
	lastSeq, err := chat.ConversationSeq(ctx, userID, conversationID)
	if err != nil {
		return cursor, err
	}
	if seq < 0 || seq > lastSeq || !ValidDeviceID(deviceID) {
		return cursor, model.Fail(400, "invalid_cursor")
	}
	cursor.LastSeq = lastSeq
	err = chat.Store.DB.QueryRow(ctx, `INSERT INTO device_cursors(user_id,conversation_id,device_id,received_seq) VALUES($1,$2,$3,$4)
		ON CONFLICT(user_id,conversation_id,device_id) DO UPDATE SET received_seq=GREATEST(device_cursors.received_seq,EXCLUDED.received_seq),updated_at=now() RETURNING received_seq`, userID, conversationID, deviceID, seq).Scan(&cursor.ReceivedSeq)
	return cursor, err
}

func ValidDeviceID(deviceID string) bool {
	if len(deviceID) < 1 || len(deviceID) > 100 {
		return false
	}
	for _, character := range deviceID {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_') {
			return false
		}
	}
	return true
}

func (chat *Chat) Cursors(ctx context.Context, userID, deviceID string) ([]model.DeviceCursor, error) {
	if !ValidDeviceID(deviceID) {
		return nil, model.Fail(400, "invalid_device")
	}
	rows, err := chat.Store.DB.Query(ctx, `SELECT cm.conversation_id,COALESCE(dc.received_seq,0),c.next_seq FROM conversation_members cm JOIN conversations c ON c.id=cm.conversation_id LEFT JOIN device_cursors dc ON dc.conversation_id=cm.conversation_id AND dc.user_id=cm.user_id AND dc.device_id=$2 WHERE cm.user_id=$1 ORDER BY cm.conversation_id`, userID, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cursors := []model.DeviceCursor{}
	for rows.Next() {
		cursor := model.DeviceCursor{DeviceID: deviceID}
		if err := rows.Scan(&cursor.ConversationID, &cursor.ReceivedSeq, &cursor.LastSeq); err != nil {
			return nil, err
		}
		cursors = append(cursors, cursor)
	}
	return cursors, rows.Err()
}
