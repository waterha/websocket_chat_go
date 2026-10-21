package model

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
