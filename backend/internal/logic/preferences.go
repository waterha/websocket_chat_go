package logic

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

// Preferences updates the caller's private view of one conversation. Pinning,
// muting and archiving are per-member, so nothing here is visible to other
// members and no event needs to be broadcast. Each field is optional: a nil
// pointer leaves the existing value untouched, which lets the client send only
// what the user changed.
func (chat *Chat) Preferences(ctx context.Context, userID, conversationID string, input model.PreferenceInput) (model.Preferences, error) {
	updated := model.Preferences{}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return updated, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)", conversationID, userID).Scan(&exists); err != nil {
		return updated, err
	}
	if !exists {
		return updated, model.Fail(404, "not_found")
	}
	if input.Pinned != nil {
		// A pin stores when it happened so several pinned conversations keep a
		// stable, meaningful order instead of an arbitrary one.
		if *input.Pinned {
			_, err = tx.Exec(ctx, "UPDATE conversation_members SET pinned_at=now() WHERE conversation_id=$1 AND user_id=$2", conversationID, userID)
		} else {
			_, err = tx.Exec(ctx, "UPDATE conversation_members SET pinned_at=NULL WHERE conversation_id=$1 AND user_id=$2", conversationID, userID)
		}
		if err != nil {
			return updated, err
		}
	}
	if input.Muted != nil {
		if _, err = tx.Exec(ctx, "UPDATE conversation_members SET muted=$1 WHERE conversation_id=$2 AND user_id=$3", *input.Muted, conversationID, userID); err != nil {
			return updated, err
		}
	}
	if input.Archived != nil {
		if *input.Archived {
			_, err = tx.Exec(ctx, "UPDATE conversation_members SET archived_at=now() WHERE conversation_id=$1 AND user_id=$2", conversationID, userID)
		} else {
			_, err = tx.Exec(ctx, "UPDATE conversation_members SET archived_at=NULL WHERE conversation_id=$1 AND user_id=$2", conversationID, userID)
		}
		if err != nil {
			return updated, err
		}
	}
	var pinnedAt, archivedAt *string
	if err = tx.QueryRow(ctx, `SELECT pinned_at::text,muted,archived_at::text FROM conversation_members WHERE conversation_id=$1 AND user_id=$2`, conversationID, userID).Scan(&pinnedAt, &updated.Muted, &archivedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return updated, model.Fail(404, "not_found")
		}
		return updated, err
	}
	updated.ConversationID = conversationID
	updated.Pinned = pinnedAt != nil
	updated.Archived = archivedAt != nil
	return updated, tx.Commit(ctx)
}
