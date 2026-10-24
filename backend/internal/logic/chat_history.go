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
	if err = chat.attachThreads(ctx, messages); err != nil {
		return nil, err
	}
	if err = chat.attachReactions(ctx, chat.Store.DB, messages); err != nil {
		return nil, err
	}
	return messages, rows.Err()
}

// attachThreads adds reply summaries to the root messages in a page. Messages
// that are themselves replies are left alone: their summary belongs to the
// window they were posted in, which the thread endpoint resolves by walking up
// to the stored root.
func (chat *Chat) attachThreads(ctx context.Context, messages []model.Message) error {
	roots := []string{}
	for _, message := range messages {
		if message.ThreadRootID == "" && message.Kind != "system" {
			roots = append(roots, message.ID)
		}
	}
	if len(roots) == 0 {
		return nil
	}
	summaries, err := chat.ThreadSummaries(ctx, roots)
	if err != nil {
		return err
	}
	for index := range messages {
		if summary, exists := summaries[messages[index].ID]; exists {
			messages[index].Thread = &summary
		}
	}
	return nil
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
	if err = chat.attachThreads(ctx, page.Messages); err != nil {
		return page, err
	}
	if err = chat.attachReactions(ctx, chat.Store.DB, page.Messages); err != nil {
		return page, err
	}
	return page, rows.Err()
}
