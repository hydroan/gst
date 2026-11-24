// Package api stands for a public package, whose exported functions a
// project calls.
package api

import "example.com/module/internal/gst"

// The constructors, forwarded as a public package forwards them to a
// project.
var (
	NewError          = gst.NewError
	NewErrorWithCause = gst.NewErrorWithCause
)

// Is reports whether err is answered with status: named as the test of an
// error against another is, it reads what the error carries.
func Is(err error, status int) bool {
	inner, ok := err.(*gst.Error)
	return ok && inner.Status() == status
}

// Check builds a refusal a project calling it may answer with, though the
// one call the module makes of it wraps what it returns.
func Check(id string) error {
	if id == "" {
		return gst.NewError(400, "the sample is nameless")
	}
	return nil
}
