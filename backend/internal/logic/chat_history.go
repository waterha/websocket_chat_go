package logic

import (
	"context"
	"slices"
	"websocket_chat_go/backend/internal/model"
)

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
