package client

import "fmt"

// Error is a structured server-side rejection: the HTTP layer answered with a
// non-2xx status, or the response envelope carried a non-zero business code.
// Transport failures (connection refused, timeout) stay ordinary errors and
// never become an *Error.
type Error struct {
	StatusCode int    // HTTP status code of the response
	Code       int    // business code from the response envelope
	Msg        string // business message from the response envelope
	TraceID    string // trace id from the response envelope
	Body       []byte // raw response body, kept for debugging
}

// Error renders the rejection as a single readable line.
func (e *Error) Error() string {
	return fmt.Sprintf("server rejected: status=%d code=%d msg=%q trace_id=%s",
		e.StatusCode, e.Code, e.Msg, e.TraceID)
}
