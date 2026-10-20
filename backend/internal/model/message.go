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

// ReplyRef is the quoted context of a replied-to message. Only a small excerpt
// is kept so replies stay readable even when the original is long or later
// recalled.
type ReplyRef struct {
	ID        string `json:"id"`
	Seq       int64  `json:"seq"`
	SenderID  string `json:"sender_id"`
	Kind      string `json:"kind"`
	Excerpt   string `json:"excerpt"`
	Recalled  bool   `json:"recalled"`
	MediaType string `json:"media_type,omitempty"`
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
	// ReplyToID is the quoted message, if any.
	ReplyToID string `json:"reply_to_id,omitempty"`
	// Reply carries the quoted excerpt so a client can render it without a
	// second request.
	Reply *ReplyRef `json:"reply,omitempty"`
	// Mentions lists the user IDs explicitly mentioned by the sender.
	Mentions []string `json:"mentions,omitempty"`
	// RecalledAt is set once the sender recalls the message. Callers must treat
	// a recalled message's content as unavailable and render a placeholder.
	RecalledAt *time.Time `json:"recalled_at,omitempty"`
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
	// ReplyToID quotes an earlier message in the same conversation.
	ReplyToID string `json:"reply_to_id"`
	// Mentions lists user IDs to notify; membership is verified by the server.
	Mentions []string `json:"mentions"`
}
