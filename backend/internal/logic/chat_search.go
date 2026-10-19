package logic

import (
	"context"
	"strings"
	"unicode/utf8"
	"websocket_chat_go/backend/internal/model"
)

func (chat *Chat) SearchMessages(ctx context.Context, userID, conversationID, query string, before int64, limit int) (model.SearchPage, error) {
	page := model.SearchPage{Messages: []model.Message{}}
	lastSeq, err := chat.ConversationSeq(ctx, userID, conversationID)
	if err != nil {
		return page, err
	}
	query = strings.TrimSpace(query)
	if length := utf8.RuneCountInString(query); length < 1 || length > 100 {
		return page, model.Fail(400, "invalid_query")
	}
	if before < 1 {
		return page, model.Fail(400, "invalid_cursor")
	}
	if limit < 1 || limit > 100 {
		return page, model.Fail(400, "invalid_limit")
	}
	// strpos treats %, _ and backslashes literally, without LIKE escaping.
	rows, err := chat.Store.DB.Query(ctx, "SELECT "+messageColumns+" FROM messages WHERE conversation_id=$1 AND seq<$2 AND seq<=$3 AND strpos(lower(content),lower($4::text))>0 ORDER BY seq DESC LIMIT $5", conversationID, before, lastSeq, query, limit+1)
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
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	page.HasMore = len(page.Messages) > limit
	if page.HasMore {
		page.Messages = page.Messages[:limit]
	}
	if len(page.Messages) > 0 {
		page.NextBeforeSeq = page.Messages[len(page.Messages)-1].Seq
	}
	return page, nil
}
