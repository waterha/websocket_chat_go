package model

type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (err *Error) Error() string { return err.Code }

func Fail(status int, code string) error {
	return &Error{Status: status, Code: code, Message: code}
}
