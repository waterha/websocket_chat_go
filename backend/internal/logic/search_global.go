package logic

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"websocket_chat_go/backend/internal/model"
)

// Global search is scoped to conversations the caller belongs to; it never
// reveals messages from other members' conversations. System notices are
// included because they carry governance history. Recalled messages are
// excluded so a withdrawn body does not resurface through search.
const globalSearchLimit = 50

// SearchAll lists matching messages across the caller's conversations. The
// before cursor is nil on the first page; pass the previous page's
// (next_before_time, next_before_id) to continue. A nil cursor must be sent as
// SQL NULL rather than a zero timestamp, otherwise every row is filtered out.
func (chat *Chat) SearchAll(ctx context.Context, userID, query string, before *time.Time, beforeID string, limit int) (model.GlobalSearchPage, error) {
	page := model.GlobalSearchPage{Results: []model.GlobalSearchHit{}}
	query = strings.TrimSpace(query)
	if length := utf8.RuneCountInString(query); length < 1 || length > 100 {
		return page, model.Fail(400, "invalid_query")
	}
	if limit < 1 || limit > globalSearchLimit {
		return page, model.Fail(400, "invalid_limit")
	}
	// A cursor is the (created_at, id) pair of the last hit. `seq` cannot be used
	// here because it is only contiguous within a single conversation, so
	// pagination across conversations needs a stable global ordering.
	var cursor any
	if before != nil {
		cursor = *before
	}
	rows, err := chat.Store.DB.Query(ctx, `SELECT `+messageColumnsFor("messages")+`, c.name, c.kind,
		(SELECT count(*) FROM conversation_members cm WHERE cm.conversation_id=messages.conversation_id)
		FROM messages
		JOIN conversation_members mine ON mine.conversation_id=messages.conversation_id AND mine.user_id=$1
		JOIN conversations c ON c.id=messages.conversation_id
		WHERE messages.recalled_at IS NULL
			AND strpos(lower(messages.content),lower($2::text))>0
			AND ($3::timestamptz IS NULL OR (messages.created_at,messages.id) < ($3,$4))
		ORDER BY messages.created_at DESC, messages.id DESC
		LIMIT $5`, userID, query, cursor, beforeID, limit+1)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	hits := []model.GlobalSearchHit{}
	for rows.Next() {
		var hit model.GlobalSearchHit
		var attachment, reply []byte
		var conversationName, conversationKind string
		var memberCount int
		message := model.Message{}
		if err = rows.Scan(&message.ID, &message.ConversationID, &message.SenderID, &message.ClientMsgID, &message.Seq, &message.Content, &message.Kind, &message.CreatedAt, &message.MediaID, &message.Mentions, &message.RecalledAt, &message.ReplyToID, &attachment, &reply, &conversationName, &conversationKind, &memberCount); err != nil {
			return page, err
		}
		if len(attachment) > 0 {
			if err = decodeJSON(attachment, &message.Media); err != nil {
				return page, err
			}
		}
		if len(reply) > 0 {
			if err = decodeJSON(reply, &message.Reply); err != nil {
				return page, err
			}
		}
		hit.Message = message
		hit.ConversationName = conversationName
		hit.ConversationKind = conversationKind
		hit.MemberCount = memberCount
		hits = append(hits, hit)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	page.HasMore = len(hits) > limit
	if page.HasMore {
		hits = hits[:limit]
	}
	page.Results = hits
	if len(hits) > 0 {
		last := hits[len(hits)-1].Message
		page.NextBeforeTime = last.CreatedAt
		page.NextBeforeID = last.ID
	}
	return page, nil
}
