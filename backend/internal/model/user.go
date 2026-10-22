package model

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// DirectoryEntry is a colleague profile shown in the organization directory.
type DirectoryEntry struct {
	User
	Department string `json:"department"`
	JobTitle   string `json:"job_title"`
}

type ProfileInput struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}
