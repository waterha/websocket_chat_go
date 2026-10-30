package logic

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

type Chat struct{ Store *model.Store }

const MaxGroupMembers = 1000

// messageColumns selects one message together with its attachment, quoted reply
// and recall state. The correlated subqueries keep every read path returning a
// complete JSON object without extra queries. media_id is coalesced because rows
// without an attachment store NULL and would otherwise scan as an error.
const messageColumns = `id,conversation_id,sender_id,client_msg_id,seq,content,kind,created_at,COALESCE(media_id,''),mentions,recalled_at,COALESCE(recalled_by,''),COALESCE((SELECT u.name FROM users u WHERE u.id=messages.recalled_by),''),COALESCE(reply_to_id,''),COALESCE(thread_root_id,''),COALESCE(blocks,'[]'::jsonb),
	(SELECT jsonb_build_object('id',mm.id,'file_name',mm.file_name,'media_type',mm.media_type,'content_type',mm.content_type,'byte_size',mm.byte_size)
	 FROM message_media mm WHERE mm.id=messages.media_id),
	(SELECT jsonb_build_object('id',r.id,'seq',r.seq,'sender_id',r.sender_id,'kind',r.kind,'recalled',r.recalled_at IS NOT NULL,
		'media_type',COALESCE((SELECT mm.media_type FROM message_media mm WHERE mm.id=r.media_id),''),
		'excerpt',CASE WHEN r.recalled_at IS NOT NULL THEN '' ELSE left(r.content,120) END)
	 FROM messages r WHERE r.id=messages.reply_to_id)`

// messageColumnsFor is messageColumns qualified with a table alias. Queries that
// join other tables must use it, otherwise the bare column names in the list are
// ambiguous (for example `id` exists on both messages and conversations).
//
// The list is assembled from parts rather than one long concatenation: the closing
// parenthesis of each COALESCE has to stay inside the SQL string, which is easy to
// get wrong in a single expression.
func messageColumnsFor(alias string) string {
	prefix := func(column string) string { return alias + "." + column }
	columns := []string{
		prefix("id"),
		prefix("conversation_id"),
		prefix("sender_id"),
		prefix("client_msg_id"),
		prefix("seq"),
		prefix("content"),
		prefix("kind"),
		prefix("created_at"),
		"COALESCE(" + prefix("media_id") + ",'')",
		prefix("mentions"),
		prefix("recalled_at"),
		"COALESCE(" + prefix("recalled_by") + ",'')",
		"COALESCE((SELECT u.name FROM users u WHERE u.id=" + prefix("recalled_by") + "),'')",
		"COALESCE(" + prefix("reply_to_id") + ",'')",
		"COALESCE(" + prefix("thread_root_id") + ",'')",
		"COALESCE(" + prefix("blocks") + ",'[]'::jsonb)",
	}
	columns = append(columns,
		`(SELECT jsonb_build_object('id',mm.id,'file_name',mm.file_name,'media_type',mm.media_type,'content_type',mm.content_type,'byte_size',mm.byte_size)
	 FROM message_media mm WHERE mm.id=`+prefix("media_id")+`)`,
		`(SELECT jsonb_build_object('id',r.id,'seq',r.seq,'sender_id',r.sender_id,'kind',r.kind,'recalled',r.recalled_at IS NOT NULL,
		'media_type',COALESCE((SELECT mm.media_type FROM message_media mm WHERE mm.id=`+prefix("media_id")+`),''),
		'excerpt',CASE WHEN r.recalled_at IS NOT NULL THEN '' ELSE left(r.content,120) END)
	 FROM messages r WHERE r.id=`+prefix("reply_to_id")+`)`,
	)
	return strings.Join(columns, ",")
}

func scanMessage(row pgx.Row) (model.Message, error) {
	var message model.Message
	var attachment, reply, blocks []byte
	err := row.Scan(&message.ID, &message.ConversationID, &message.SenderID, &message.ClientMsgID, &message.Seq, &message.Content, &message.Kind, &message.CreatedAt, &message.MediaID, &message.Mentions, &message.RecalledAt, &message.RecalledBy, &message.RecalledByName, &message.ReplyToID, &message.ThreadRootID, &blocks, &attachment, &reply)
	if err != nil {
		return message, err
	}
	if len(attachment) > 0 {
		if err := decodeJSON(attachment, &message.Media); err != nil {
			return message, err
		}
	}
	if len(reply) > 0 {
		if err := decodeJSON(reply, &message.Reply); err != nil {
			return message, err
		}
	}
	if len(blocks) > 0 {
		if err := decodeJSON(blocks, &message.Blocks); err != nil {
			return message, err
		}
		// An empty array and a missing one mean the same to a client, so an empty
		// slice is normalised away and the original rendering path applies.
		if len(message.Blocks) == 0 {
			message.Blocks = nil
		}
	}
	// A recalled message hides everything about its body. MediaID is cleared too:
	// leaving it in the payload would let a reader fetch the bytes that the
	// recall was meant to withdraw, since the download endpoint authorises by
	// conversation membership alone. Blocks go with it for the same reason: every
	// image block names an attachment that the recall withdraws.
	if message.RecalledAt != nil {
		message.Content = ""
		message.Media = nil
		message.MediaID = ""
		message.Blocks = nil
	}
	return message, nil
}

// decodeJSON unmarshals a jsonb column into a pointer target.
func decodeJSON(payload []byte, target any) error {
	return json.Unmarshal(payload, target)
}

// insertMessage writes one message and re-reads it inside the same transaction.
// The read uses messageColumns, because PostgreSQL RETURNING cannot evaluate the
// correlated attachment and reply subqueries that every other read path relies on.
func insertMessage(ctx context.Context, tx pgx.Tx, input model.Message) (model.Message, error) {
	identifier := model.NewID()
	mentions := input.Mentions
	if mentions == nil {
		mentions = []string{}
	}
	var blocks any
	if len(input.Blocks) > 0 {
		encoded, err := json.Marshal(input.Blocks)
		if err != nil {
			return model.Message{}, err
		}
		blocks = encoded
	}
	if _, err := tx.Exec(ctx, `INSERT INTO messages(id,conversation_id,sender_id,client_msg_id,seq,content,kind,media_id,reply_to_id,thread_root_id,mentions,blocks)
		VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),$11,$12::jsonb)`, identifier, input.ConversationID, input.SenderID, input.ClientMsgID, input.Seq, input.Content, input.Kind, input.MediaID, input.ReplyToID, input.ThreadRootID, mentions, blocks); err != nil {
		return model.Message{}, err
	}
	return scanMessage(tx.QueryRow(ctx, "SELECT "+messageColumns+" FROM messages WHERE id=$1", identifier))
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
