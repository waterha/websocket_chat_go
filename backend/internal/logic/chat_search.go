package logic

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
	"websocket_chat_go/backend/internal/model"
)

// SearchFilters narrows a conversation search. From is inclusive and To is
// exclusive; nil boundaries are unbounded. Type is empty/all, images, links or files.
type SearchFilters struct {
	SenderID string
	Type     string
	From     *time.Time
	To       *time.Time
}

func (chat *Chat) SearchMessages(ctx context.Context, userID, conversationID, query string, before int64, limit int, filters ...SearchFilters) (model.SearchPage, error) {
	page := model.SearchPage{Messages: []model.Message{}}
	lastSeq, err := chat.ConversationSeq(ctx, userID, conversationID)
	if err != nil {
		return page, err
	}
	query = strings.TrimSpace(query)
	if length := utf8.RuneCountInString(query); length > 100 {
		return page, model.Fail(400, "invalid_query")
	}
	var filter SearchFilters
	if len(filters) > 1 {
		return page, model.Fail(400, "invalid_filters")
	}
	if len(filters) == 1 {
		filter = filters[0]
	}
	switch filter.Type {
	case "", "all", "images", "links", "files":
	default:
		return page, model.Fail(400, "invalid_type")
	}
	if filter.From != nil && filter.To != nil && !filter.From.Before(*filter.To) {
		return page, model.Fail(400, "invalid_time_range")
	}
	if before < 1 {
		return page, model.Fail(400, "invalid_cursor")
	}
	if limit < 1 || limit > 100 {
		return page, model.Fail(400, "invalid_limit")
	}
	// PostgreSQL stores microseconds. Round both bounds up to the next stored
	// instant so pgx truncation cannot include a row before From or exclude one
	// that is strictly before a nanosecond-precision To.
	storedBoundary := func(boundary *time.Time) *time.Time {
		if boundary == nil {
			return nil
		}
		value := boundary.Truncate(time.Microsecond)
		if value.Before(*boundary) {
			value = value.Add(time.Microsecond)
		}
		return &value
	}
	filter.From, filter.To = storedBoundary(filter.From), storedBoundary(filter.To)
	// strpos treats %, _ and backslashes literally, without LIKE escaping.
	// Content predicates must not reveal a recalled body's former contents.
	// Inspect only text blocks for links, never image metadata such as file_name.
	rows, err := chat.Store.DB.Query(ctx, "SELECT "+messageColumns+` FROM messages
		WHERE conversation_id=$1 AND seq<$2 AND seq<=$3
		AND ($4::text='' OR (recalled_at IS NULL AND strpos(lower(content),lower($4::text))>0))
		AND ($6::text='' OR sender_id=$6)
		AND ($7::timestamptz IS NULL OR created_at >= $7)
		AND ($8::timestamptz IS NULL OR created_at < $8)
		AND (CASE $9::text
			WHEN 'images' THEN recalled_at IS NULL AND (kind='image' OR EXISTS (
				SELECT 1 FROM jsonb_array_elements(COALESCE(blocks,'[]'::jsonb)) b WHERE b->>'type'='image'))
			WHEN 'files' THEN recalled_at IS NULL AND kind='file'
			WHEN 'links' THEN recalled_at IS NULL AND (content ~* '(https?://|www[.])' OR EXISTS (
				SELECT 1 FROM jsonb_array_elements(COALESCE(blocks,'[]'::jsonb)) b
				WHERE b->>'type'='text' AND b->>'text' ~* '(https?://|www[.])'))
			ELSE TRUE END)
		ORDER BY seq DESC LIMIT $5`, conversationID, before, lastSeq, query, limit+1, filter.SenderID, filter.From, filter.To, filter.Type)
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
