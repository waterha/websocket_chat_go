package model

import "time"

// Member roles inside a group conversation. Direct conversations keep every
// member as 'member' and have no owner.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

type RemoveMemberInput struct {
	UserID string `json:"user_id"`
}

type SetRoleInput struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

type TransferInput struct {
	UserID string `json:"user_id"`
}

// MemberAliasInput sets the group card shown for one member.
type MemberAliasInput struct {
	Alias string `json:"alias"`
}

// MemberMuteInput mutes a member for the given number of minutes; zero
// minutes lifts the mute.
type MemberMuteInput struct {
	Minutes int `json:"minutes"`
}

// ConversationSettingsInput carries the group-wide toggles an admin controls.
type ConversationSettingsInput struct {
	// AllMuted silences every plain member until it is turned off.
	AllMuted *bool `json:"all_muted"`
	// JoinApproval routes new invites through an admin decision.
	JoinApproval *bool `json:"join_approval"`
}

// PokeInput names the member to tap on the shoulder.
type PokeInput struct {
	UserID string `json:"user_id"`
}

// ConversationSettings reports the group-wide toggles now in force.
type ConversationSettings struct {
	AllMuted     bool `json:"all_muted"`
	JoinApproval bool `json:"join_approval"`
}

// JoinRequest is one pending invite awaiting an admin decision.
type JoinRequest struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	UserID         string    `json:"user_id"`
	Message        string    `json:"message"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	User           User      `json:"user"`
}

// JoinRequestDecision is the outcome of one decided request.
type JoinRequestDecision struct {
	JoinRequest JoinRequest `json:"join_request"`
	Accepted    bool        `json:"accepted"`
}
