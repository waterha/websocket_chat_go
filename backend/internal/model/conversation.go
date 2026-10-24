package model

import (
	"time"
)

type Member struct {
	User
	Role        string     `json:"role"`
	LastReadSeq int64      `json:"last_read_seq"`
	ReadAt      *time.Time `json:"read_at"`
	// Online reflects a live WebSocket connection within the heartbeat window.
	// It is a best-effort hint, not a delivery guarantee.
	Online bool `json:"online"`
	// Alias is the member's group card, shown inside this conversation only.
	Alias string `json:"alias"`
	// MutedUntil is set when an admin silences this member; nil means the
	// member may speak.
	MutedUntil *time.Time `json:"muted_until"`
}

type Conversation struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	OwnerID string `json:"owner_id,omitempty"`
	// AnnouncementCount lets the panel show how many notices exist without
	// fetching them for every conversation in the sidebar.
	AnnouncementCount int      `json:"announcement_count"`
	Members           []Member `json:"members"`
	LastSeq           int64    `json:"last_seq"`
	LastReadSeq       int64    `json:"last_read_seq"`
	// These reflect only the caller's own preferences and view state.
	Pinned    bool       `json:"pinned"`
	Muted     bool       `json:"muted"`
	Alias     string     `json:"alias"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	// Group-wide governance state.
	AllMuted     bool `json:"all_muted"`
	JoinApproval bool `json:"join_approval"`
	Dismissed    bool `json:"dismissed"`
	// PendingJoins counts the join requests awaiting a decision, so the
	// sidebar can badge groups an admin has to look at.
	PendingJoins int `json:"pending_joins"`
}

type CreateInput struct {
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	MemberIDs []string `json:"member_ids"`
}

type AddMembersInput struct {
	MemberIDs []string `json:"member_ids"`
}

// AddMembersResult reports whether the invite went straight through or is
// waiting for an admin to approve it.
type AddMembersResult struct {
	ConversationID string `json:"id"`
	Pending        bool   `json:"pending"`
}

// DeleteConversationInput is reserved for future options; the call already
// names the conversation in the path.
type DeleteConversationInput struct{}
