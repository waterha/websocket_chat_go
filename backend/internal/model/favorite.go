package model

import "time"

// Favorite is an immutable, server-captured message owned by one user.
type Favorite struct {
	ID               string    `json:"id"`
	Message          Message   `json:"message"`
	SenderName       string    `json:"sender_name"`
	SenderAvatar     string    `json:"sender_avatar"`
	ConversationName string    `json:"conversation_name"`
	ConversationKind string    `json:"conversation_kind"`
	SavedAt          time.Time `json:"saved_at"`
	IsOwn            bool      `json:"is_own"`
}

type FavoritePage struct {
	Favorites    []Favorite `json:"favorites"`
	HasMore      bool       `json:"has_more"`
	NextBeforeID string     `json:"next_before_id"`
}
