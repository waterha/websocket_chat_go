package model

// Preferences is one member's private view of a conversation. It is not shared
// with other members and does not change conversation behaviour for them.
type Preferences struct {
	ConversationID string `json:"conversation_id"`
	Pinned         bool   `json:"pinned"`
	Muted          bool   `json:"muted"`
}

// PreferenceInput uses pointers so an omitted field means "leave unchanged"
// rather than "set to false".
type PreferenceInput struct {
	Pinned *bool `json:"pinned"`
	Muted  *bool `json:"muted"`
}
