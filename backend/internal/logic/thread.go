package logic

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

// ThreadPreviewLimit is how many replies the main chat view shows under a root
// message before the user opens the whole window.
const ThreadPreviewLimit = 2

// ThreadReplyLimit bounds one page of a thread window.
const ThreadReplyLimit = 100

// Thread returns one page of a message's window, oldest first, together with the
// root message and the total reply count. Only conversation members may read it,
// which ConversationSeq enforces.
func (chat *Chat) Thread(ctx context.Context, userID, conversationID, rootID string, after int64, limit int) (model.ThreadPage, error) {
	page := model.ThreadPage{Replies: []model.Message{}}
	if limit < 1 || limit > ThreadReplyLimit {
		return page, model.Fail(400, "invalid_limit")
	}
	if after < 0 {
		return page, model.Fail(400, "invalid_cursor")
	}
	lastSeq, err := chat.ConversationSeq(ctx, userID, conversationID)
	if err != nil {
		return page, err
	}
	root, err := chat.loadMessage(ctx, conversationID, rootID)
	if err != nil {
		return page, err
	}
	// A reply addresses its window through the stored root, so navigating from a
	// reply opens the same window instead of a nested one.
	if root.ThreadRootID != "" {
		root, err = chat.loadMessage(ctx, conversationID, root.ThreadRootID)
		if err != nil {
			return page, err
		}
	}
	if err = chat.Store.DB.QueryRow(ctx, "SELECT count(*) FROM messages WHERE thread_root_id=$1", root.ID).Scan(&page.ReplyCount); err != nil {
		return page, err
	}
	rows, err := chat.Store.DB.Query(ctx, "SELECT "+messageColumns+" FROM messages WHERE thread_root_id=$1 AND seq>$2 AND seq<=$3 ORDER BY seq LIMIT $4", root.ID, after, lastSeq, limit+1)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	replies := []model.Message{}
	for rows.Next() {
		reply, scanErr := scanMessage(rows)
		if scanErr != nil {
			return page, scanErr
		}
		replies = append(replies, reply)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	page.HasMore = len(replies) > limit
	if page.HasMore {
		replies = replies[:limit]
	}
	page.Replies = replies
	page.NextAfter = after
	if len(replies) > 0 {
		page.NextAfter = replies[len(replies)-1].Seq
	}
	page.Root = root
	// Reactions live in their own table and are attached per page, keeping the
	// hot message read paths free of correlated aggregates.
	if err = chat.attachReactions(ctx, chat.Store.DB, page.Replies); err != nil {
		return page, err
	}
	withRoot := []model.Message{page.Root}
	if err = chat.attachReactions(ctx, chat.Store.DB, withRoot); err != nil {
		return page, err
	}
	page.Root = withRoot[0]
	return page, nil
}

// loadMessage reads one message in a conversation, mapping a missing row to 404.
func (chat *Chat) loadMessage(ctx context.Context, conversationID, messageID string) (model.Message, error) {
	message, err := scanMessage(chat.Store.DB.QueryRow(ctx, "SELECT "+messageColumns+" FROM messages WHERE id=$1 AND conversation_id=$2", messageID, conversationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Message{}, model.Fail(404, "not_found")
	}
	return message, err
}

// ThreadSummaries returns reply counts and the first few replies for the given
// root messages, keyed by message id. It uses one query for the whole page
// because the chat view asks for every visible message at once, and a query per
// message would scale with the page size.
func (chat *Chat) ThreadSummaries(ctx context.Context, messageIDs []string) (map[string]model.ThreadSummary, error) {
	summaries := map[string]model.ThreadSummary{}
	if len(messageIDs) == 0 {
		return summaries, nil
	}
	rows, err := chat.Store.DB.Query(ctx, `SELECT thread_root_id,count(*)::int FROM messages
		WHERE thread_root_id=ANY($1) GROUP BY thread_root_id`, messageIDs)
	if err != nil {
		return summaries, err
	}
	for rows.Next() {
		var rootID string
		var count int
		if err = rows.Scan(&rootID, &count); err != nil {
			rows.Close()
			return summaries, err
		}
		summaries[rootID] = model.ThreadSummary{RootID: rootID, ReplyCount: count, Preview: []model.Message{}}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return summaries, err
	}
	if len(summaries) == 0 {
		return summaries, nil
	}
	// The preview is the oldest few replies, matching the order of the window so
	// the summary reads as the start of the conversation rather than its tail.
	// The column list must be qualified with the same alias the query uses.
	previewRows, err := chat.Store.DB.Query(ctx, `SELECT `+messageColumnsFor("m")+`,m.thread_root_id FROM messages m
		WHERE m.thread_root_id=ANY($1)
			AND (SELECT count(*) FROM messages earlier WHERE earlier.thread_root_id=m.thread_root_id AND earlier.seq<m.seq) < $2
		ORDER BY m.thread_root_id,m.seq`, messageIDs, ThreadPreviewLimit)
	if err != nil {
		return summaries, err
	}
	defer previewRows.Close()
	for previewRows.Next() {
		reply, scanErr := scanMessageWithRoot(previewRows)
		if scanErr != nil {
			return summaries, scanErr
		}
		summary := summaries[reply.threadRoot]
		summary.Preview = append(summary.Preview, reply.message)
		summaries[reply.threadRoot] = summary
	}
	return summaries, previewRows.Err()
}

// rootedMessage pairs a scanned message with the thread root held in the same
// row, so the preview query can group replies without a second lookup.
type rootedMessage struct {
	message    model.Message
	threadRoot string
}

// scanMessageWithRoot reads messageColumnsFor plus the trailing thread_root_id.
func scanMessageWithRoot(row pgx.Row) (rootedMessage, error) {
	var rooted rootedMessage
	var attachment, reply, blocks []byte
	err := row.Scan(&rooted.message.ID, &rooted.message.ConversationID, &rooted.message.SenderID, &rooted.message.ClientMsgID, &rooted.message.Seq, &rooted.message.Content, &rooted.message.Kind, &rooted.message.CreatedAt, &rooted.message.MediaID, &rooted.message.Mentions, &rooted.message.RecalledAt, &rooted.message.ReplyToID, &rooted.message.ThreadRootID, &blocks, &attachment, &reply, &rooted.threadRoot)
	if err != nil {
		return rooted, err
	}
	if len(blocks) > 0 {
		if err := decodeJSON(blocks, &rooted.message.Blocks); err != nil {
			return rooted, err
		}
		if len(rooted.message.Blocks) == 0 {
			rooted.message.Blocks = nil
		}
	}
	if len(attachment) > 0 {
		if err := decodeJSON(attachment, &rooted.message.Media); err != nil {
			return rooted, err
		}
	}
	if len(reply) > 0 {
		if err := decodeJSON(reply, &rooted.message.Reply); err != nil {
			return rooted, err
		}
	}
	if rooted.message.RecalledAt != nil {
		rooted.message.Content = ""
		rooted.message.Media = nil
		rooted.message.MediaID = ""
	}
	return rooted, nil
}
