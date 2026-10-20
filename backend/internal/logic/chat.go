package logic

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

type Chat struct{ Store *model.Store }

const MaxGroupMembers = 1000

// messageColumns selects one message together with its attachment, quoted reply
// and recall state. The correlated subqueries keep every read path returning a
// complete JSON object without extra queries. media_id is coalesced because rows
// without an attachment store NULL and would otherwise scan as an error.
const messageColumns = `id,conversation_id,sender_id,client_msg_id,seq,content,kind,created_at,COALESCE(media_id,''),mentions,recalled_at,COALESCE(reply_to_id,''),
	(SELECT jsonb_build_object('id',mm.id,'file_name',mm.file_name,'media_type',mm.media_type,'content_type',mm.content_type,'byte_size',mm.byte_size)
	 FROM message_media mm WHERE mm.id=messages.media_id),
	(SELECT jsonb_build_object('id',r.id,'seq',r.seq,'sender_id',r.sender_id,'kind',r.kind,'recalled',r.recalled_at IS NOT NULL,
		'media_type',COALESCE((SELECT mm.media_type FROM message_media mm WHERE mm.id=r.media_id),''),
		'excerpt',CASE WHEN r.recalled_at IS NOT NULL THEN '' ELSE left(r.content,120) END)
	 FROM messages r WHERE r.id=messages.reply_to_id)`

func scanMessage(row pgx.Row) (model.Message, error) {
	var message model.Message
	var attachment, reply []byte
	err := row.Scan(&message.ID, &message.ConversationID, &message.SenderID, &message.ClientMsgID, &message.Seq, &message.Content, &message.Kind, &message.CreatedAt, &message.MediaID, &message.Mentions, &message.RecalledAt, &message.ReplyToID, &attachment, &reply)
	if err != nil {
		return message, err
	}
	if len(attachment) > 0 {
		var media model.Media
		if err := json.Unmarshal(attachment, &media); err != nil {
			return message, err
		}
		message.Media = &media
	}
	if len(reply) > 0 {
		var quoted model.ReplyRef
		if err := json.Unmarshal(reply, &quoted); err != nil {
			return message, err
		}
		message.Reply = &quoted
	}
	// A recalled message hides its body, matching what the quoted excerpt shows.
	if message.RecalledAt != nil {
		message.Content = ""
		message.Media = nil
	}
	return message, nil
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
	if _, err := tx.Exec(ctx, `INSERT INTO messages(id,conversation_id,sender_id,client_msg_id,seq,content,kind,media_id,reply_to_id,mentions)
		VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10)`, identifier, input.ConversationID, input.SenderID, input.ClientMsgID, input.Seq, input.Content, input.Kind, input.MediaID, input.ReplyToID, mentions); err != nil {
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
