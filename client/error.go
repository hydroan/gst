package client

import "fmt"

// Error is a structured server-side rejection: the HTTP layer answered with a
// non-2xx status. Transport failures (connection refused, timeout) stay
// ordinary errors and never become an *Error.
type Error struct {
	StatusCode      int              // HTTP status code of the response
	Msg             string           // message from the response envelope
	TraceID         string           // trace id from the response envelope
	FieldViolations []FieldViolation // fields the request was refused on, from the data of the response envelope; nil when the rejection names none
	Body            []byte           // raw response body, kept for debugging
}

// FieldViolation is one field the server refused a request on: the field by
// its JSON key path, address.city or items[1].name, and the description of
// what it failed.
type FieldViolation struct {
	Field       string `json:"field"`
	Description string `json:"description"`
}

// Error renders the rejection as a single readable line.
func (e *Error) Error() string {
	return fmt.Sprintf("server rejected: status=%d msg=%q trace_id=%s",
		e.StatusCode, e.Msg, e.TraceID)
}
