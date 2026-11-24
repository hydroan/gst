// Package types stands for the package declaring the error a request is
// answered with.
package gst

// Error stands for the error a request is answered with.
type Error struct {
	status int
	msg    string
	cause  error
}

// Error returns the sentence.
func (e *Error) Error() string { return e.msg }

// Status returns the status a client is answered with.
func (e *Error) Status() int { return e.status }

// Msg returns the sentence a client is answered with.
func (e *Error) Msg() string { return e.msg }

// NewError takes the sentence as its second parameter.
func NewError(status int, msg string) *Error { return &Error{status: status, msg: msg} }

// NewErrorWithCause takes the sentence as its second parameter, and keeps
// cause without reading it.
func NewErrorWithCause(status int, msg string, cause error) *Error {
	return &Error{status: status, msg: msg, cause: cause}
}
