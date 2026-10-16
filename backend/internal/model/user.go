package model

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type ProfileInput struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}
