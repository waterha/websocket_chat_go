package logic

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

const friendRequestColumns = `fr.id,fr.requester_id,fr.recipient_id,fr.message,fr.status,fr.created_at,fr.updated_at,
	requester.id,requester.email,requester.name,COALESCE(requester.avatar_media_id,''),recipient.id,recipient.email,recipient.name,COALESCE(recipient.avatar_media_id,'')`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanFriendRequest(row rowScanner) (model.FriendRequest, error) {
	var request model.FriendRequest
	err := row.Scan(
		&request.ID, &request.RequesterID, &request.RecipientID, &request.Message, &request.Status, &request.CreatedAt, &request.UpdatedAt,
		&request.Requester.ID, &request.Requester.Email, &request.Requester.Name, &request.Requester.Avatar,
		&request.Recipient.ID, &request.Recipient.Email, &request.Recipient.Name, &request.Recipient.Avatar,
	)
	return request, err
}

func friendRequestQuery() string {
	return "SELECT " + friendRequestColumns + " FROM friend_requests fr JOIN users requester ON requester.id=fr.requester_id JOIN users recipient ON recipient.id=fr.recipient_id "
}

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

func (auth *Auth) FriendRequests(ctx context.Context, userID string) (model.FriendRequestList, error) {
	rows, err := auth.Store.DB.Query(ctx, friendRequestQuery()+`WHERE (fr.recipient_id=$1 AND fr.status='pending') OR fr.requester_id=$1 ORDER BY fr.updated_at DESC,fr.id`, userID)
	if err != nil {
		return model.FriendRequestList{}, err
	}
	defer rows.Close()
	requests := model.FriendRequestList{Incoming: []model.FriendRequest{}, Outgoing: []model.FriendRequest{}}
	for rows.Next() {
		request, scanErr := scanFriendRequest(rows)
		if scanErr != nil {
			return model.FriendRequestList{}, scanErr
		}
		if request.RecipientID == userID {
			requests.Incoming = append(requests.Incoming, request)
		} else {
			requests.Outgoing = append(requests.Outgoing, request)
		}
	}
	return requests, rows.Err()
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

func (auth *Auth) CreateFriendRequest(ctx context.Context, userID string, input model.AddFriendInput) (model.FriendRequest, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.Message = strings.TrimSpace(input.Message)
	if input.UserID == "" || input.UserID == userID || len([]rune(input.Message)) > 500 {
		return model.FriendRequest{}, model.Fail(400, "invalid_friend_request")
	}
	var exists bool
	if err := auth.Store.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)", input.UserID).Scan(&exists); err != nil {
		return model.FriendRequest{}, err
	}
	if !exists {
		return model.FriendRequest{}, model.Fail(404, "not_found")
	}
	if err := auth.Store.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM friendships WHERE user_id=$1 AND friend_id=$2)", userID, input.UserID).Scan(&exists); err != nil {
		return model.FriendRequest{}, err
	}
	if exists {
		return model.FriendRequest{}, model.Fail(409, "already_friends")
	}
	// A block list is personal: the recipient decides who may approach them,
	// and the caller's own block stops them from reaching out as well.
	if blocked, err := blockedBetween(ctx, auth.Store.DB, input.UserID, userID); err != nil {
		return model.FriendRequest{}, err
	} else if blocked {
		return model.FriendRequest{}, model.Fail(403, "blocked_by_user")
	}
	if blocked, err := blockedBetween(ctx, auth.Store.DB, userID, input.UserID); err != nil {
		return model.FriendRequest{}, err
	} else if blocked {
		return model.FriendRequest{}, model.Fail(400, "user_blocked")
	}
	var existingID string
	err := auth.Store.DB.QueryRow(ctx, "SELECT id FROM friend_requests WHERE requester_id=$1 AND recipient_id=$2 AND status='pending'", userID, input.UserID).Scan(&existingID)
	if err == nil {
		return auth.friendRequest(ctx, existingID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.FriendRequest{}, err
	}
	if err := auth.Store.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM friend_requests WHERE requester_id=$1 AND recipient_id=$2 AND status='pending')", input.UserID, userID).Scan(&exists); err != nil {
		return model.FriendRequest{}, err
	}
	if exists {
		return model.FriendRequest{}, model.Fail(409, "request_exists")
	}
	var requestID string
	err = auth.Store.DB.QueryRow(ctx, `INSERT INTO friend_requests(id,requester_id,recipient_id,message) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING RETURNING id`, model.NewID(), userID, input.UserID, input.Message).Scan(&requestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.FriendRequest{}, model.Fail(409, "request_exists")
	}
	if err != nil {
		return model.FriendRequest{}, err
	}
	return auth.friendRequest(ctx, requestID)
}

func (auth *Auth) friendRequest(ctx context.Context, requestID string) (model.FriendRequest, error) {
	return scanFriendRequest(auth.Store.DB.QueryRow(ctx, friendRequestQuery()+`WHERE fr.id=$1`, requestID))
}

func (auth *Auth) DecideFriendRequest(ctx context.Context, userID, requestID, status string) (model.FriendRequest, error) {
	if status != "accepted" && status != "rejected" {
		return model.FriendRequest{}, model.Fail(400, "invalid_friend_request_action")
	}
	tx, err := auth.Store.DB.Begin(ctx)
	if err != nil {
		return model.FriendRequest{}, err
	}
	defer tx.Rollback(ctx)
	var requesterID, recipientID string
	err = tx.QueryRow(ctx, `SELECT requester_id,recipient_id FROM friend_requests WHERE id=$1 AND recipient_id=$2 AND status='pending' FOR UPDATE`, requestID, userID).Scan(&requesterID, &recipientID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.FriendRequest{}, model.Fail(404, "not_found")
	}
	if err != nil {
		return model.FriendRequest{}, err
	}
	if status == "accepted" {
		if blocked, blockErr := blockedBetween(ctx, tx, recipientID, requesterID); blockErr != nil {
			return model.FriendRequest{}, blockErr
		} else if blocked {
			return model.FriendRequest{}, model.Fail(403, "blocked_by_user")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO friendships(user_id,friend_id) VALUES($1,$2),($2,$1) ON CONFLICT DO NOTHING`, requesterID, recipientID); err != nil {
			return model.FriendRequest{}, err
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE friend_requests SET status=$1,updated_at=now() WHERE id=$2", status, requestID); err != nil {
		return model.FriendRequest{}, err
	}
	request, err := scanFriendRequest(tx.QueryRow(ctx, friendRequestQuery()+`WHERE fr.id=$1`, requestID))
	if err != nil {
		return model.FriendRequest{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return model.FriendRequest{}, err
	}
	return request, nil
}
