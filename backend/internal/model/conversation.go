package model

import (
	"time"
)

type Member struct {
	User
	Role        string     `json:"role"`
	LastReadSeq int64      `json:"last_read_seq"`
	ReadAt      *time.Time `json:"read_at"`
}

type Conversation struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Announcement string   `json:"announcement"`
	OwnerID      string   `json:"owner_id,omitempty"`
	Members      []Member `json:"members"`
	LastSeq      int64    `json:"last_seq"`
	LastReadSeq  int64    `json:"last_read_seq"`
}

type CreateInput struct {
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	MemberIDs []string `json:"member_ids"`
}

type AddMembersInput struct {
	MemberIDs []string `json:"member_ids"`
}
