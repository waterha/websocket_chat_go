package logic

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"websocket_chat_go/backend/internal/model"
)

// Global search is scoped to conversations the caller belongs to; it never
// reveals messages from other members' conversations. System notices are
// included because they carry governance history. Recalled messages are
// excluded so a withdrawn body does not resurface through search.
const globalSearchLimit = 50

// Keep filtering, membership and keyset ordering inside the bounded candidate
// query; hydrate attachments and count each result conversation only afterwards.
// Partial expression-index selectivity can fall back to PostgreSQL's 0.5%
// default even for ubiquitous grams. Use its collected element frequencies to
// avoid expanding huge GIN posting lists for hot terms. Both branches retain
// exactly the same literal-match semantics; absent/stale statistics affect cost
// only. Do not put metadata projections before the candidate LIMIT.
func globalSearchSQL(hasCursor bool) string {
	cursor := ""
	if hasCursor {
		cursor = " AND (m.created_at,m.id) < ($4::timestamptz,$5::text)"
	}
	return `WITH term_stats AS MATERIALIZED (
		SELECT most_common_elems::text::text[] AS elems,most_common_elem_freqs AS freqs
		FROM pg_stats WHERE tablename='messages_global_search_grams_idx'
		AND schemaname=(SELECT n.nspname FROM pg_class r JOIN pg_namespace n ON n.oid=r.relnamespace WHERE r.oid='messages'::regclass)
	), strategy AS MATERIALIZED (
		SELECT COALESCE(min(COALESCE(freqs[array_position(elems,gram)],0)),0)>=0.20 AS hot
		FROM unnest(message_search_query_grams($2::text)) gram LEFT JOIN term_stats ON true
	), matched AS MATERIALIZED (
		SELECT id,conversation_id,created_at FROM (
		 (SELECT m.id,m.conversation_id,m.created_at FROM messages m
		 JOIN conversation_members mine ON mine.conversation_id=m.conversation_id AND mine.user_id=$1
		 WHERE (SELECT hot FROM strategy) AND m.recalled_at IS NULL
		 AND strpos(lower(m.content),lower($2::text))>0` + cursor + `
		 ORDER BY m.created_at DESC,m.id DESC LIMIT $3)
		 UNION ALL
		 (SELECT m.id,m.conversation_id,m.created_at FROM messages m
		 JOIN conversation_members mine ON mine.conversation_id=m.conversation_id AND mine.user_id=$1
		 WHERE NOT (SELECT hot FROM strategy) AND m.recalled_at IS NULL
		 AND message_search_grams(m.content) @> message_search_query_grams($2::text)
		 AND strpos(lower(m.content),lower($2::text))>0` + cursor + `
		 ORDER BY m.created_at DESC,m.id DESC LIMIT $3)
		) candidates ORDER BY created_at DESC,id DESC LIMIT $3
	), counts AS MATERIALIZED (
		SELECT ids.conversation_id,(SELECT count(*) FROM conversation_members cm WHERE cm.conversation_id=ids.conversation_id) AS member_count
		FROM (SELECT DISTINCT conversation_id FROM matched) ids
	)
	SELECT ` + messageColumnsFor("messages") + `,c.name,c.kind,counts.member_count
	FROM matched JOIN messages ON messages.id=matched.id
	JOIN conversations c ON c.id=matched.conversation_id
	JOIN counts ON counts.conversation_id=matched.conversation_id
	ORDER BY matched.created_at DESC,matched.id DESC`
}

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
	// Plan against the actual term/cursor rather than a cached generic plan:
	// common one-character queries and rare long terms have different costs.
	args := []any{pgx.QueryExecModeExec, userID, query, limit + 1}
	if before != nil {
		args = append(args, *before, beforeID)
	}
	rows, err := chat.Store.DB.Query(ctx, globalSearchSQL(before != nil), args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	hits := []model.GlobalSearchHit{}
	for rows.Next() {
		var hit model.GlobalSearchHit
		var attachment, reply, blocks []byte
		var conversationName, conversationKind string
		var memberCount int
		message := model.Message{}
		if err = rows.Scan(&message.ID, &message.ConversationID, &message.SenderID, &message.ClientMsgID, &message.Seq, &message.Content, &message.Kind, &message.CreatedAt, &message.MediaID, &message.Mentions, &message.RecalledAt, &message.RecalledBy, &message.RecalledByName, &message.ReplyToID, &message.ThreadRootID, &blocks, &attachment, &reply, &conversationName, &conversationKind, &memberCount); err != nil {
			return page, err
		}
		if len(blocks) > 0 {
			if err = decodeJSON(blocks, &message.Blocks); err != nil {
				return page, err
			}
			if len(message.Blocks) == 0 {
				message.Blocks = nil
			}
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
