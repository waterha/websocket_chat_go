package model

import (
	"time"
)

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	SenderID       string    `json:"sender_id"`
	ClientMsgID    string    `json:"client_msg_id"`
	Seq            int64     `json:"seq"`
	Content        string    `json:"content"`
	Kind           string    `json:"kind"`
	CreatedAt      time.Time `json:"created_at"`
}

type SearchPage struct {
	Messages      []Message `json:"messages"`
	HasMore       bool      `json:"has_more"`
	NextBeforeSeq int64     `json:"next_before_seq"`
}

type SendInput struct {
	ConversationID string `json:"conversation_id"`
	ClientMsgID    string `json:"client_msg_id"`
	Content        string `json:"content"`
}
