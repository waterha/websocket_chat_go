package model

import "time"

type Sticker struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	FileName  string    `json:"file_name"`
	ByteSize  int64     `json:"byte_size"`
	CreatedAt time.Time `json:"created_at"`
}

type StickerPage struct {
	Stickers []Sticker `json:"stickers"`
}
