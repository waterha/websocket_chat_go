package model

import "time"

type AddFriendInput struct {
	UserID  string `json:"user_id"`
	Message string `json:"message"`
}

type FriendRequest struct {
	ID          string    `json:"id"`
	RequesterID string    `json:"requester_id"`
	RecipientID string    `json:"recipient_id"`
	Message     string    `json:"message"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Requester   User      `json:"requester"`
	Recipient   User      `json:"recipient"`
}

type FriendRequestList struct {
	Incoming []FriendRequest `json:"incoming"`
	Outgoing []FriendRequest `json:"outgoing"`
}
