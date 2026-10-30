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

// ThreadSummary lets the main chat view show a compact preview under a message
// without fetching the whole thread. Replies are truncated to a few previews so
// a busy thread does not inflate every message in the history.
type ThreadSummary struct {
	RootID      string    `json:"root_id"`
	ReplyCount  int       `json:"reply_count"`
	Preview     []Message `json:"preview"`
	LastReplyAt time.Time `json:"last_reply_at,omitempty"`
}

// ThreadPage is one page of a thread window, ordered oldest first so the window
// reads like a conversation.
type ThreadPage struct {
	Root       Message   `json:"root"`
	Replies    []Message `json:"replies"`
	HasMore    bool      `json:"has_more"`
	NextAfter  int64     `json:"next_after_seq"`
	ReplyCount int       `json:"reply_count"`
}

// Block is one ordered segment of a message. A message can interleave text and
// images so the bubble matches the order the sender typed.
//
// Blocks are stored on the message so a reader needs no extra request. The media
// metadata is denormalised into the block because an uploaded attachment's name,
// type and size never change after the upload.
type Block struct {
	// Type is "text" or "image".
	Type string `json:"type"`
	// Text holds the segment for a text block.
	Text string `json:"text,omitempty"`
	// MediaID references an attachment for an image block. The server fills the
	// remaining media fields itself; a client-supplied value for them is ignored.
	MediaID     string `json:"media_id,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	MediaType   string `json:"media_type,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	ByteSize    int64  `json:"byte_size,omitempty"`
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
	// Blocks holds the message as an ordered list of text and image segments. It is
	// empty for messages sent before blocks existed and for a plain text or single
	// attachment message, which clients render from content and media instead.
	Blocks []Block `json:"blocks,omitempty"`
	// Reply carries the quoted excerpt so a client can render it without a
	// second request.
	Reply *ReplyRef `json:"reply,omitempty"`
	// ThreadRootID is set on a reply and names the message whose window it
	// belongs to. It is empty on a root message and on messages without replies.
	ThreadRootID string `json:"thread_root_id,omitempty"`
	// Thread summarises replies to this message. It is present only on the root
	// messages that the caller is currently reading.
	Thread *ThreadSummary `json:"thread,omitempty"`
	// Mentions lists the user IDs explicitly mentioned by the sender.
	Mentions []string `json:"mentions,omitempty"`
	// RecalledAt is set once a sender or moderator recalls the message. Callers
	// must treat its content as unavailable and render a placeholder.
	RecalledAt *time.Time `json:"recalled_at,omitempty"`
	// RecalledBy identifies the first recall's actor; legacy recalls may omit it.
	RecalledBy string `json:"recalled_by,omitempty"`
	// RecalledByName is the actor's current name, even after leaving the group.
	RecalledByName string `json:"recalled_by_name,omitempty"`
	// Reactions aggregates the emoji responses the message received. Each entry
	// lists every reacting user so a client can highlight its own reaction.
	Reactions []Reaction `json:"reactions,omitempty"`
}

// Reaction is one emoji with everyone who reacted to the message with it.
type Reaction struct {
	Emoji string   `json:"emoji"`
	Count int      `json:"count"`
	Users []string `json:"users"`
}

// ReactionInput names the emoji to add or remove. The caller is taken from the
// session.
type ReactionInput struct {
	Emoji string `json:"emoji"`
}

type SearchPage struct {
	Messages      []Message `json:"messages"`
	HasMore       bool      `json:"has_more"`
	NextBeforeSeq int64     `json:"next_before_seq"`
}

// GlobalSearchHit is one match in the caller's conversations, carrying enough
// conversation context to render and navigate without further requests.
type GlobalSearchHit struct {
	Message          Message `json:"message"`
	ConversationName string  `json:"conversation_name"`
	ConversationKind string  `json:"conversation_kind"`
	MemberCount      int     `json:"member_count"`
}

type GlobalSearchPage struct {
	Results        []GlobalSearchHit `json:"results"`
	HasMore        bool              `json:"has_more"`
	NextBeforeTime time.Time         `json:"next_before_time"`
	NextBeforeID   string            `json:"next_before_id"`
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
	// Blocks carries a message whose images are interleaved with its text. When it
	// is set the plain content is derived from it, so a client sending blocks does
	// not also need to send content.
	Blocks []Block `json:"blocks"`
	// ThreadRootID names the window a reply belongs to. A client may set it when
	// replying inside a window; the server derives it when omitted, so a reply
	// to a reply always joins the original window.
	ThreadRootID string `json:"thread_root_id"`
	// Mentions lists user IDs to notify; membership is verified by the server.
	Mentions []string `json:"mentions"`
}
