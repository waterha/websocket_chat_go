package model

import "time"

// Announcement is one notice published in a group. Owners and admins create,
// edit and delete them; every member reads the whole list.
type Announcement struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	AuthorID       string    `json:"author_id"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	// Edited tells the client whether the notice was changed after publication,
	// so it can show that instead of comparing timestamps itself.
	Edited bool `json:"edited"`
}

// AnnouncementInput carries the text for a new or edited notice. The id comes
// from the path, and the author always comes from the session.
type AnnouncementInput struct {
	Content string `json:"content"`
}
