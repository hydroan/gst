package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/errorstack"
	"github.com/stretchr/testify/require"
)

func TestNewErrorStackTraceStartsAtPublicConstructionSite(t *testing.T) {
	err := newSamplePublicStackError()

	stackTrace := errorstack.Origin(err)
	require.NotEmpty(t, stackTrace)

	// The public package forwards to serviceregistry without adding a stack
	// frame, so the innermost frame must be the application construction
	// site, not a framework wrapper.
	lines := strings.Split(stackTrace, "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	require.Contains(t, lines[0], "newSamplePublicStackError")
	require.Contains(t, lines[1], "error_test.go")
}

func TestNewErrorWithCauseStackTraceStartsAtPublicConstructionSite(t *testing.T) {
	err := newSamplePublicStackErrorWithPlainCause()

	stackTrace := errorstack.Origin(err)
	require.NotEmpty(t, stackTrace)

	// The plain cause carries no stack trace of its own, so the reported
	// origin falls back to the service error construction site.
	lines := strings.Split(stackTrace, "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	require.Contains(t, lines[0], "newSamplePublicStackErrorWithPlainCause")
	require.Contains(t, lines[1], "error_test.go")
}

// newSamplePublicStackError constructs a service error through the public
// constructor inside a dedicated helper, so tests can assert the captured
// stack points at this construction site.
func newSamplePublicStackError() *Error {
	return NewError(http.StatusConflict, "sample record missing")
}

// newSamplePublicStackErrorWithPlainCause wraps a cause without an embedded
// stack trace, so the construction-site stack is the only origin candidate.
func newSamplePublicStackErrorWithPlainCause() *Error {
	return NewErrorWithCause(http.StatusInternalServerError, "sample load failed", samplePlainCauseError{})
}

// samplePlainCauseError is a cause error without any embedded stack trace.
type samplePlainCauseError struct{}

func (samplePlainCauseError) Error() string { return "sample plain cause" }
