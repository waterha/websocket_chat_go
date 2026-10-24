package model

type User struct {
	ID string `json:"id"`
	// Avatar references an upload owned by the user; an empty value means the
	// client renders the initial-letter placeholder.
	Avatar string `json:"avatar,omitempty"`
	Email  string `json:"email"`
	Name   string `json:"name"`
}

// DirectoryEntry is a colleague profile shown in the organization directory.
type DirectoryEntry struct {
	User
	Department string `json:"department"`
	JobTitle   string `json:"job_title"`
}

type WorkProfile struct {
	Department string `json:"department"`
	JobTitle   string `json:"job_title"`
}

type DirectoryPage struct {
	Entries     []DirectoryEntry `json:"entries"`
	Departments []string         `json:"departments"`
	HasMore     bool             `json:"has_more"`
	NextOffset  int              `json:"next_offset"`
}

type ProfileInput struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}
