package logic

import (
	"context"
	"websocket_chat_go/backend/internal/model"
)

func (auth *Auth) Friends(ctx context.Context, userID string) ([]model.User, error) {
	rows, err := auth.Store.DB.Query(ctx, `SELECT u.id,u.email,u.name FROM friendships f JOIN users u ON u.id=f.friend_id WHERE f.user_id=$1 ORDER BY u.name,u.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	friends := []model.User{}
	for rows.Next() {
		var friend model.User
		if err := rows.Scan(&friend.ID, &friend.Email, &friend.Name); err != nil {
			return nil, err
		}
		friends = append(friends, friend)
	}
	return friends, rows.Err()
}

func (auth *Auth) AddFriend(ctx context.Context, userID string, input model.AddFriendInput) error {
	if input.UserID == "" || input.UserID == userID {
		return model.Fail(400, "invalid_friend")
	}
	tx, err := auth.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)", input.UserID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return model.Fail(404, "not_found")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO friendships(user_id,friend_id) VALUES($1,$2),($2,$1) ON CONFLICT DO NOTHING`, userID, input.UserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
