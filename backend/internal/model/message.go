package model

import (
	"time"
)

// Media is the copy of an upload embedded into a message. Storage keys are
// deliberately absent from JSON: a client fetches bytes only through the
// authorised download endpoint.
type Media struct {
	ID          string `json:"id"`
	FileName    string `json:"file_name"`
	MediaType   string `json:"media_type"`
	ContentType string `json:"content_type"`
	ByteSize    int64  `json:"byte_size"`
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	SenderID       string    `json:"sender_id"`
	ClientMsgID    string    `json:"client_msg_id"`
	Seq            int64     `json:"seq"`
	Content        string    `json:"content"`
	Kind           string    `json:"kind"`
	CreatedAt      time.Time `json:"created_at"`
	MediaID        string    `json:"media_id,omitempty"`
	Media          *Media    `json:"media,omitempty"`
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
	// MediaID attaches an already uploaded image or video. When present the
	// message kind is derived from that upload instead of the caller's input.
	MediaID string `json:"media_id"`
}
