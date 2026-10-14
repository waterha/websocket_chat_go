package model

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
