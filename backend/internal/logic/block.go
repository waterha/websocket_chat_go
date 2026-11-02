package logic

import (
	"context"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

// Blocks lists the caller's blocked users.
func (auth *Auth) Blocks(ctx context.Context, userID string) ([]model.BlockEntry, error) {
	rows, err := auth.Store.DB.Query(ctx, `SELECT u.id,u.email,u.name,COALESCE(u.avatar_media_id,''),b.created_at FROM user_blocks b JOIN users u ON u.id=b.blocked_user_id WHERE b.user_id=$1 ORDER BY b.created_at DESC,u.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	blocks := []model.BlockEntry{}
	for rows.Next() {
		var entry model.BlockEntry
		if err := rows.Scan(&entry.ID, &entry.Email, &entry.Name, &entry.Avatar, &entry.CreatedAt); err != nil {
			return nil, err
		}
		blocks = append(blocks, entry)
	}
	return blocks, rows.Err()
}

// Block adds a user to the caller's block list. Blocking is idempotent and the
// friendship is not touched: unblocking restores exactly what was there.
func (auth *Auth) Block(ctx context.Context, userID, targetID string) error {
	if targetID == "" || targetID == userID {
		return model.Fail(400, "invalid_block")
	}
	var exists bool
	if err := auth.Store.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)", targetID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return model.Fail(404, "not_found")
	}
	_, err := auth.Store.DB.Exec(ctx, "INSERT INTO user_blocks(user_id,blocked_user_id) VALUES($1,$2) ON CONFLICT DO NOTHING", userID, targetID)
	return err
}

// Unblock removes a user from the caller's block list.
func (auth *Auth) Unblock(ctx context.Context, userID, targetID string) error {
	tag, err := auth.Store.DB.Exec(ctx, "DELETE FROM user_blocks WHERE user_id=$1 AND blocked_user_id=$2", userID, targetID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.Fail(404, "not_found")
	}
	return nil
}

// querier is the subset of *pgxpool.Pool and pgx.Tx the block helpers need.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// blockedBetween reports whether blocker has blocked user. Transaction-aware so
// the send and create gates can check inside their own transaction.
func blockedBetween(ctx context.Context, db querier, blocker, user string) (bool, error) {
	var blocked bool
	err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM user_blocks WHERE user_id=$1 AND blocked_user_id=$2)", blocker, user).Scan(&blocked)
	return blocked, err
}
