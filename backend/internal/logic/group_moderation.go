package logic

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

// maxMuteMinutes bounds a timed mute to thirty days.
const maxMuteMinutes = 30 * 24 * 60

// SetMemberMute silences one member for the given minutes, or lifts the mute
// when minutes is zero. An admin may only mute plain members; the owner may
// also mute admins, never another owner.
func (chat *Chat) SetMemberMute(ctx context.Context, callerID, conversationID, targetID string, minutes int) error {
	if targetID == "" || minutes < 0 || minutes > maxMuteMinutes {
		return model.Fail(400, "invalid_mute")
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	callerRole, err := chat.roleOf(ctx, tx, callerID, conversationID)
	if err != nil {
		return err
	}
	if callerRole == model.RoleMember {
		return model.Fail(403, "forbidden")
	}
	var targetRole string
	err = tx.QueryRow(ctx, "SELECT role FROM conversation_members WHERE conversation_id=$1 AND user_id=$2", conversationID, targetID).Scan(&targetRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Fail(404, "not_found")
	}
	if err != nil {
		return err
	}
	if targetRole == model.RoleOwner || (callerRole == model.RoleAdmin && targetRole == model.RoleAdmin) {
		return model.Fail(403, "forbidden")
	}
	var mutedUntil any
	notice := ""
	if minutes > 0 {
		mutedUntil = time.Now().Add(time.Duration(minutes) * time.Minute)
		var targetName, callerName string
		if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", targetID).Scan(&targetName); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", callerID).Scan(&callerName); err != nil {
			return err
		}
		notice = callerName + "禁言了" + targetName + " " + strconv.Itoa(minutes) + " 分钟"
	} else {
		var targetName, callerName string
		if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", targetID).Scan(&targetName); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", callerID).Scan(&callerName); err != nil {
			return err
		}
		notice = callerName + "解除了对" + targetName + "的禁言"
	}
	if _, err = tx.Exec(ctx, "UPDATE conversation_members SET muted_until=$1 WHERE conversation_id=$2 AND user_id=$3", mutedUntil, conversationID, targetID); err != nil {
		return err
	}
	if err = chat.systemNotice(ctx, tx, conversationID, callerID, notice); err != nil {
		return err
	}
	if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
