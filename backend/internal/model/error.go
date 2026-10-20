package model

import "time"

type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	// RetryAfter is set on throttling responses so the handler can send a
	// Retry-After header without leaking the value into the JSON body.
	RetryAfter time.Duration `json:"-"`
}

func (err *Error) Error() string { return err.Code }

// WithRetry attaches a suggested wait to a throttling error. It is a no-op on
// the plain error interface, so callers should prefer FailRetry when the static
// type matters.
func (err *Error) WithRetry(after time.Duration) *Error {
	err.RetryAfter = after
	return err
}

func Fail(status int, code string) error {
	return &Error{Status: status, Code: code, Message: code}
}

// FailRetry is Fail with a suggested wait, used for throttling responses.
func FailRetry(status int, code string, after time.Duration) error {
	return &Error{Status: status, Code: code, Message: code, RetryAfter: after}
}
