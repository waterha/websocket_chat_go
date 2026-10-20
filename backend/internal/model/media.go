package model

import "time"

// MediaUpload is one stored image or video. StorageKey is the on-disk name and
// is never exposed through JSON: clients fetch bytes from the authorised
// download endpoint using ID only.
type MediaUpload struct {
	ID             string    `json:"id"`
	OwnerID        string    `json:"-"`
	ConversationID string    `json:"conversation_id"`
	FileName       string    `json:"file_name"`
	MediaType      string    `json:"media_type"`
	ContentType    string    `json:"content_type"`
	ByteSize       int64     `json:"byte_size"`
	StorageKey     string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
}
