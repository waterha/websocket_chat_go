package logic

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

const friendRequestColumns = `fr.id,fr.requester_id,fr.recipient_id,fr.message,fr.status,fr.created_at,fr.updated_at,
	requester.id,requester.email,requester.name,recipient.id,recipient.email,recipient.name`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanFriendRequest(row rowScanner) (model.FriendRequest, error) {
	var request model.FriendRequest
	err := row.Scan(
		&request.ID, &request.RequesterID, &request.RecipientID, &request.Message, &request.Status, &request.CreatedAt, &request.UpdatedAt,
		&request.Requester.ID, &request.Requester.Email, &request.Requester.Name,
		&request.Recipient.ID, &request.Recipient.Email, &request.Recipient.Name,
	)
	return request, err
}

func friendRequestQuery() string {
	return "SELECT " + friendRequestColumns + " FROM friend_requests fr JOIN users requester ON requester.id=fr.requester_id JOIN users recipient ON recipient.id=fr.recipient_id "
}

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
