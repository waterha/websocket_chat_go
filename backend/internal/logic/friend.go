package logic

import (
	"context"
	"strings"

	"websocket_chat_go/backend/internal/model"
)

func (auth *Auth) Friends(ctx context.Context, userID string) ([]model.Friend, error) {
	rows, err := auth.Store.DB.Query(ctx, `SELECT u.id,u.email,u.name,COALESCE(u.avatar_media_id,''),f.remark FROM friendships f JOIN users u ON u.id=f.friend_id WHERE f.user_id=$1 ORDER BY u.name,u.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	friends := []model.Friend{}
	for rows.Next() {
		var friend model.Friend
		if err := rows.Scan(&friend.ID, &friend.Email, &friend.Name, &friend.Avatar, &friend.Remark); err != nil {
			return nil, err
		}
		friends = append(friends, friend)
	}
	return friends, rows.Err()
}

// maxFriendRemarkRunes keeps the private note short enough for a list row.
const maxFriendRemarkRunes = 50

// SetFriendRemark updates the caller's private note for an existing friend.
func (auth *Auth) SetFriendRemark(ctx context.Context, userID, friendID, remark string) (model.Friend, error) {
	remark = strings.TrimSpace(remark)
	if len([]rune(remark)) > maxFriendRemarkRunes {
		return model.Friend{}, model.Fail(400, "invalid_friend_remark")
	}
	tag, err := auth.Store.DB.Exec(ctx, "UPDATE friendships SET remark=$1 WHERE user_id=$2 AND friend_id=$3", remark, userID, friendID)
	if err != nil {
		return model.Friend{}, err
	}
	if tag.RowsAffected() == 0 {
		return model.Friend{}, model.Fail(404, "not_found")
	}
	var friend model.Friend
	err = auth.Store.DB.QueryRow(ctx, `SELECT u.id,u.email,u.name,COALESCE(u.avatar_media_id,''),f.remark FROM friendships f JOIN users u ON u.id=f.friend_id WHERE f.user_id=$1 AND f.friend_id=$2`, userID, friendID).Scan(&friend.ID, &friend.Email, &friend.Name, &friend.Avatar, &friend.Remark)
	return friend, err
}

// DeleteFriend removes the friendship in both directions. The conversations the
// two users shared stay untouched: history belongs to the conversation, not to
// the friendship.
func (auth *Auth) DeleteFriend(ctx context.Context, userID, friendID string) error {
	tag, err := auth.Store.DB.Exec(ctx, "DELETE FROM friendships WHERE user_id=$1 AND friend_id=$2", userID, friendID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.Fail(404, "not_found")
	}
	_, err = auth.Store.DB.Exec(ctx, "DELETE FROM friendships WHERE user_id=$1 AND friend_id=$2", friendID, userID)
	return err
}
