package gst

import "github.com/hydroan/gst/internal/types"

// Error is the error a request is answered with: a status and a client-safe
// message, with an internal cause for the logs. A service method, a model
// hook, a middleware or an interceptor returns it to refuse a request with a
// status of its own; any other error is the server's failure, answered 500.
type Error = types.Error

// The constructors are forwarded as variables instead of wrapper functions
// on purpose: a wrapper function would add its own frame on top of the
// stack trace captured at the construction site.
var (
	// NewError creates the error a request is answered with: status and a
	// client-safe message.
	//
	// The status must be a 4xx or 5xx HTTP status code. Invalid statuses, including
	// 2xx/3xx success or redirect statuses such as http.StatusOK, are normalized to
	// http.StatusInternalServerError and the provided message is discarded.
	NewError = types.NewError

	// NewErrorWithCause creates the error a request is answered with, carrying
	// an internal cause.
	//
	// The status must be a 4xx or 5xx HTTP status code. Invalid statuses, including
	// 2xx/3xx success or redirect statuses such as http.StatusOK, are normalized to
	// http.StatusInternalServerError and the provided message is discarded.
	//
	// The cause is reported by Error for logs and available through Unwrap, but
	// is never exposed as the response message.
	NewErrorWithCause = types.NewErrorWithCause
)
