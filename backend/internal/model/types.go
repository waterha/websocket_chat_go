package model

import "time"

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	SenderID       string    `json:"sender_id"`
	ClientMsgID    string    `json:"client_msg_id"`
	Seq            int64     `json:"seq"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}

type Member struct {
	User
	LastReadSeq int64      `json:"last_read_seq"`
	ReadAt      *time.Time `json:"read_at"`
}

type Conversation struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Members     []Member `json:"members"`
	LastSeq     int64    `json:"last_seq"`
	LastReadSeq int64    `json:"last_read_seq"`
}

type SendInput struct {
	ConversationID string `json:"conversation_id"`
	ClientMsgID    string `json:"client_msg_id"`
	Content        string `json:"content"`
}

type CreateInput struct {
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	MemberIDs []string `json:"member_ids"`
}

type Receipt struct {
	ConversationID string     `json:"conversation_id"`
	UserID         string     `json:"user_id"`
	Seq            int64      `json:"seq"`
	ReadAt         *time.Time `json:"read_at"`
}

type DeviceCursor struct {
	ConversationID string `json:"conversation_id"`
	DeviceID       string `json:"device_id"`
	ReceivedSeq    int64  `json:"received_seq"`
}

type SyncPage struct {
	Messages []Message `json:"messages"`
	LastSeq  int64     `json:"last_seq"`
	NextSeq  int64     `json:"next_seq"`
	HasMore  bool      `json:"has_more"`
}

type Event struct {
	Event     string `json:"event"`
	RequestID string `json:"request_id,omitempty"`
	Data      any    `json:"data,omitempty"`
	Error     *Error `json:"error,omitempty"`
}

type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (err *Error) Error() string { return err.Code }

func Fail(status int, code string) error {
	return &Error{Status: status, Code: code, Message: code}
}
