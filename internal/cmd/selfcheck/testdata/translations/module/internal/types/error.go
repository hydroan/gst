// Package types stands for the package declaring the error a request is
// answered with.
package types

// Error stands for the error a request is answered with.
type Error struct{ msg string }

// NewError takes the sentence as its second parameter.
func NewError(status int, msg string) *Error { return &Error{msg: msg} }

// NewErrorWithCause takes the sentence as its second parameter.
func NewErrorWithCause(status int, msg string, cause error) *Error { return &Error{msg: msg} }
