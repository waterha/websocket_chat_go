package logic

import (
	"context"

	"websocket_chat_go/backend/internal/model"
)

// Settings flips the group-wide toggles. Only admins and the owner may change
// them, and the response reports the values now in force.
func (chat *Chat) Settings(ctx context.Context, callerID, conversationID string, input model.ConversationSettingsInput) (model.ConversationSettings, error) {
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return model.ConversationSettings{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = chat.roleOf(ctx, tx, callerID, conversationID); err != nil {
		return model.ConversationSettings{}, err
	}
	if input.AllMuted != nil {
		if _, err = tx.Exec(ctx, "UPDATE conversations SET all_muted=$1 WHERE id=$2", *input.AllMuted, conversationID); err != nil {
			return model.ConversationSettings{}, err
		}
	}
	if input.JoinApproval != nil {
		if _, err = tx.Exec(ctx, "UPDATE conversations SET join_approval=$1 WHERE id=$2", *input.JoinApproval, conversationID); err != nil {
			return model.ConversationSettings{}, err
		}
	}
	var settings model.ConversationSettings
	if err = tx.QueryRow(ctx, "SELECT all_muted,join_approval FROM conversations WHERE id=$1", conversationID).Scan(&settings.AllMuted, &settings.JoinApproval); err != nil {
		return model.ConversationSettings{}, err
	}
	if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
		return model.ConversationSettings{}, err
	}
	return settings, tx.Commit(ctx)
}

// DismissGroup closes a group for good: only the owner may do it, history stays
// readable, and no further messages are accepted.
func (chat *Chat) DismissGroup(ctx context.Context, callerID, conversationID string) error {
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	callerRole, err := chat.roleOf(ctx, tx, callerID, conversationID)
	if err != nil {
		return err
	}
	if callerRole != model.RoleOwner {
		return model.Fail(403, "forbidden")
	}
	var dismissed bool
	if err = tx.QueryRow(ctx, "SELECT dismissed_at IS NOT NULL FROM conversations WHERE id=$1", conversationID).Scan(&dismissed); err != nil {
		return err
	}
	if dismissed {
		return model.Fail(409, "already_dismissed")
	}
	if _, err = tx.Exec(ctx, "UPDATE conversations SET dismissed_at=now() WHERE id=$1", conversationID); err != nil {
		return err
	}
	if err = chat.systemNotice(ctx, tx, conversationID, callerID, "群主解散了群聊"); err != nil {
		return err
	}
	if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
