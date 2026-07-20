package service

import "github.com/hydroan/gst/internal/serviceregistry"

// Error represents a service-layer error that can be converted to an API response.
type Error = serviceregistry.Error

// The constructors are forwarded as variables instead of wrapper functions
// on purpose: a wrapper function would add its own frame on top of the
// stack trace captured at the construction site.
var (
	// NewError creates a service-layer error with a client-safe message.
	//
	// The status must be a 4xx or 5xx HTTP status code. Invalid statuses, including
	// 2xx/3xx success or redirect statuses such as http.StatusOK, are normalized to
	// http.StatusInternalServerError and the provided message is discarded.
	NewError = serviceregistry.NewError

	// NewErrorWithCause creates a service-layer error with an internal cause.
	//
	// The status must be a 4xx or 5xx HTTP status code. Invalid statuses, including
	// 2xx/3xx success or redirect statuses such as http.StatusOK, are normalized to
	// http.StatusInternalServerError and the provided message is discarded.
	//
	// The cause is available through Unwrap but is never exposed as the response message.
	NewErrorWithCause = serviceregistry.NewErrorWithCause
)
