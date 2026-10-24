package model

import "time"

// Friend is one entry of the caller's friend list. The remark is the caller's
// private note for this friend and is shown instead of the account name when
// it is set.
type Friend struct {
	User
	Remark string `json:"remark"`
}

type AddFriendInput struct {
	UserID  string `json:"user_id"`
	Message string `json:"message"`
}

// FriendRemarkInput renames a friend locally.
type FriendRemarkInput struct {
	Remark string `json:"remark"`
}

// BlockEntry is one user on the caller's block list.
type BlockEntry struct {
	User
	CreatedAt time.Time `json:"created_at"`
}

// BlockInput names the user to block or unblock.
type BlockInput struct {
	UserID string `json:"user_id"`
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
