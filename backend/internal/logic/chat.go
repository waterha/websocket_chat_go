package logic

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

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

func (chat *Chat) ConversationSeq(ctx context.Context, userID, conversationID string) (int64, error) {
	var seq int64
	err := chat.Store.DB.QueryRow(ctx, `SELECT c.next_seq FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id WHERE c.id=$1 AND cm.user_id=$2`, conversationID, userID).Scan(&seq)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, model.Fail(404, "not_found")
	}
	return seq, err
}

func (chat *Chat) History(ctx context.Context, userID, conversationID string, before int64, limit int) ([]model.Message, error) {
	lastSeq, err := chat.ConversationSeq(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	if before < 1 || limit < 1 || limit > 100 {
		return nil, model.Fail(400, "invalid_cursor")
	}
	rows, err := chat.Store.DB.Query(ctx, "SELECT "+messageColumns+" FROM messages WHERE conversation_id=$1 AND seq<$2 AND seq<=$3 ORDER BY seq DESC LIMIT $4", conversationID, before, lastSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []model.Message{}
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	slices.Reverse(messages)
	return messages, rows.Err()
}

func (chat *Chat) Sync(ctx context.Context, userID, conversationID string, after int64, limit int) (model.SyncPage, error) {
	page := model.SyncPage{Messages: []model.Message{}, NextSeq: after}
	lastSeq, err := chat.ConversationSeq(ctx, userID, conversationID)
	if err != nil {
		return page, err
	}
	if after < 0 || after > lastSeq || limit < 1 || limit > 100 {
		return page, model.Fail(400, "invalid_cursor")
	}
	page.LastSeq = lastSeq
	rows, err := chat.Store.DB.Query(ctx, "SELECT "+messageColumns+" FROM messages WHERE conversation_id=$1 AND seq>$2 AND seq<=$3 ORDER BY seq LIMIT $4", conversationID, after, lastSeq, limit)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return page, err
		}
		page.Messages = append(page.Messages, message)
		page.NextSeq = message.Seq
	}
	page.HasMore = page.NextSeq < lastSeq
	return page, rows.Err()
}

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

func (chat *Chat) Conversations(ctx context.Context, userID string) ([]model.Conversation, error) {
	rows, err := chat.Store.DB.Query(ctx, `SELECT c.id,c.name,c.kind,c.next_seq,cm.last_read_seq,
		(SELECT jsonb_agg(jsonb_build_object('id',u.id,'name',u.name,'email',u.email,'last_read_seq',members.last_read_seq,'read_at',members.read_at) ORDER BY u.name,u.id) FROM conversation_members members JOIN users u ON u.id=members.user_id WHERE members.conversation_id=c.id)
		FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id WHERE cm.user_id=$1 ORDER BY c.created_at DESC,c.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	conversations := []model.Conversation{}
	for rows.Next() {
		var item model.Conversation
		var members []byte
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.LastSeq, &item.LastReadSeq, &members); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(members, &item.Members); err != nil {
			return nil, err
		}
		conversations = append(conversations, item)
	}
	return conversations, rows.Err()
}

func (chat *Chat) Create(ctx context.Context, userID string, input model.CreateInput) (string, error) {
	input.Name = strings.TrimSpace(input.Name)
	if (input.Kind != "direct" && input.Kind != "group") || (input.Kind == "direct" && len(input.MemberIDs) != 1) || (input.Kind == "group" && (len(input.MemberIDs) < 1 || len(input.MemberIDs) >= MaxGroupMembers || len([]rune(input.Name)) < 1 || len([]rune(input.Name)) > 100)) {
		return "", model.Fail(400, "invalid_conversation")
	}
	ids := append([]string{userID}, input.MemberIDs...)
	slices.Sort(ids)
	for index, memberID := range ids {
		if memberID == "" || index > 0 && memberID == ids[index-1] {
			return "", model.Fail(400, "invalid_members")
		}
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE id=ANY($1)", ids).Scan(&count); err != nil {
		return "", err
	}
	if count != len(ids) {
		return "", model.Fail(400, "invalid_members")
	}
	conversationID := model.NewID()
	var directKey any
	if input.Kind == "direct" {
		directKey = strings.Join(ids, ":")
		input.Name = ""
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "direct:"+directKey.(string)); err != nil {
			return "", err
		}
		var existing string
		err = tx.QueryRow(ctx, "SELECT id FROM conversations WHERE direct_key=$1", directKey).Scan(&existing)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO conversations(id,name,kind,direct_key) VALUES($1,$2,$3,$4)", conversationID, input.Name, input.Kind, directKey); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO conversation_members(conversation_id,user_id) SELECT $1,unnest($2::text[])", conversationID, ids); err != nil {
		return "", err
	}
	if err = chat.appendEvent(ctx, tx, conversationID, model.Event{Event: "conversation.created", Data: map[string]string{"id": conversationID}}); err != nil {
		return "", err
	}
	return conversationID, tx.Commit(ctx)
}
