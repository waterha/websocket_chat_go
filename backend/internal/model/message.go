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
	CreatedAt      time.Time `json:"created_at"`
}

type SendInput struct {
	ConversationID string `json:"conversation_id"`
	ClientMsgID    string `json:"client_msg_id"`
	Content        string `json:"content"`
}
