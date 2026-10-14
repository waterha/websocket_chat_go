package model

import (
	"time"
)

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
	LastSeq        int64  `json:"last_seq"`
}

type DeliveryStatus struct {
	ConversationID   string    `json:"conversation_id"`
	Seq              int64     `json:"seq"`
	ConfirmedMembers int       `json:"confirmed_members"`
	TotalMembers     int       `json:"total_members"`
	AsOf             time.Time `json:"as_of"`
}
