package logic

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

// maxReactionRunes bounds one emoji response; a single grapheme is normal and
// a short ZWJ sequence still fits.
const maxReactionRunes = 16

// reactionQuerier is the subset of *pgxpool.Pool and pgx.Tx the attachment pass
// needs, so it can run inside the transaction that just changed the rows.
type reactionQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// react toggles or removes the caller's emoji response to one message and
// returns the message with its new reaction summary, which also travels to
// every member as a message.updated event.
func (chat *Chat) react(ctx context.Context, userID, conversationID, messageID, emoji string, add bool) (model.Message, error) {
	if emoji == "" || len([]rune(emoji)) > maxReactionRunes {
		return model.Message{}, model.Fail(400, "invalid_reaction")
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return model.Message{}, err
	}
	defer tx.Rollback(ctx)
	var member bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)", conversationID, userID).Scan(&member); err != nil {
		return model.Message{}, err
	}
	if !member {
		return model.Message{}, model.Fail(404, "not_found")
	}
	var recalled bool
	err = tx.QueryRow(ctx, "SELECT recalled_at IS NOT NULL FROM messages WHERE id=$1 AND conversation_id=$2", messageID, conversationID).Scan(&recalled)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Message{}, model.Fail(404, "not_found")
	}
	if err != nil {
		return model.Message{}, err
	}
	if recalled {
		return model.Message{}, model.Fail(409, "message_recalled")
	}
	if add {
		if _, err = tx.Exec(ctx, "INSERT INTO message_reactions(message_id,user_id,emoji) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", messageID, userID, emoji); err != nil {
			return model.Message{}, err
		}
	} else if _, err = tx.Exec(ctx, "DELETE FROM message_reactions WHERE message_id=$1 AND user_id=$2 AND emoji=$3", messageID, userID, emoji); err != nil {
		return model.Message{}, err
	}
	message, err := scanMessage(tx.QueryRow(ctx, "SELECT "+messageColumns+" FROM messages WHERE id=$1", messageID))
	if err != nil {
		return model.Message{}, err
	}
	updated := []model.Message{message}
	if err = chat.attachReactions(ctx, tx, updated); err != nil {
		return model.Message{}, err
	}
	message = updated[0]
	if err = chat.appendEvent(ctx, tx, conversationID, model.Event{Event: "message.updated", Data: message}); err != nil {
		return model.Message{}, err
	}
	return message, tx.Commit(ctx)
}

// attachReactions fills the reaction summary for a batch of messages with one
// grouped query. It stays out of messageColumns on purpose: a correlated
// aggregate per row would put a sort node into every history page plan, and
// most messages carry no reactions at all.
func (chat *Chat) attachReactions(ctx context.Context, db reactionQuerier, messages []model.Message) error {
	if len(messages) == 0 {
		return nil
	}
	ids := make([]string, 0, len(messages))
	byID := make(map[string][]*model.Message, len(messages))
	for index := range messages {
		ids = append(ids, messages[index].ID)
		byID[messages[index].ID] = append(byID[messages[index].ID], &messages[index])
	}
	rows, err := db.Query(ctx, `SELECT message_id,emoji,count(*),jsonb_agg(user_id ORDER BY user_id)
		FROM message_reactions WHERE message_id=ANY($1) GROUP BY message_id,emoji ORDER BY message_id`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var messageID, emoji string
		var count int
		var users []string
		if err := rows.Scan(&messageID, &emoji, &count, &users); err != nil {
			return err
		}
		for _, message := range byID[messageID] {
			message.Reactions = append(message.Reactions, model.Reaction{Emoji: emoji, Count: count, Users: users})
		}
	}
	return rows.Err()
}

func (chat *Chat) React(ctx context.Context, userID, conversationID, messageID, emoji string) (model.Message, error) {
	return chat.react(ctx, userID, conversationID, messageID, emoji, true)
}

func (chat *Chat) Unreact(ctx context.Context, userID, conversationID, messageID, emoji string) (model.Message, error) {
	return chat.react(ctx, userID, conversationID, messageID, emoji, false)
}
