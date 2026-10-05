package client

import "fmt"

// Error is a structured server-side rejection: the HTTP layer answered with a
// non-2xx status. Transport failures (connection refused, timeout) stay
// ordinary errors and never become an *Error.
type Error struct {
	StatusCode int    // HTTP status code of the response
	Msg        string // message from the response envelope
	TraceID    string // trace id from the response envelope
	Body       []byte // raw response body, kept for debugging
}

// Error renders the rejection as a single readable line.
func (e *Error) Error() string {
	return fmt.Sprintf("server rejected: status=%d msg=%q trace_id=%s",
		e.StatusCode, e.Msg, e.TraceID)
}
