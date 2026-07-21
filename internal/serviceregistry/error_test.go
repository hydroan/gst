package serviceregistry

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/errorstack"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

func TestNewError(t *testing.T) {
	err := NewError(http.StatusBadRequest, "invalid input")

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, err.Status())
	require.Equal(t, -1, err.Code())
	require.Equal(t, "invalid input", err.Msg())
	require.Equal(t, "invalid input", err.Error())

	var coder types.Coder = err
	require.Equal(t, err.Status(), coder.Status())
	require.Equal(t, err.Code(), coder.Code())
	require.Equal(t, err.Msg(), coder.Msg())
}

func TestNewErrorNormalizesInvalidStatus(t *testing.T) {
	for _, status := range []int{0, http.StatusOK, http.StatusFound, 99, 600} {
		err := NewError(status, "should not leak")

		require.Equal(t, http.StatusInternalServerError, err.Status())
		require.Equal(t, -1, err.Code())
		require.Equal(t, http.StatusText(http.StatusInternalServerError), err.Msg())
	}
}

func TestNewErrorUsesHTTPStatusTextWhenMessageIsEmpty(t *testing.T) {
	err := NewError(http.StatusNotFound, "")

	require.Equal(t, http.StatusNotFound, err.Status())
	require.Equal(t, -1, err.Code())
	require.Equal(t, http.StatusText(http.StatusNotFound), err.Msg())
}

func TestNewErrorWithCauseIncludesCauseInErrorButNotMsg(t *testing.T) {
	cause := errors.New("database password leaked")
	err := NewErrorWithCause(http.StatusInternalServerError, "failed to load user", cause)

	require.ErrorIs(t, err, cause)
	// Msg stays client-safe: the response envelope renders Msg, never Error.
	require.Equal(t, "failed to load user", err.Msg())
	require.NotContains(t, err.Msg(), cause.Error())
	// Error reports the full chain so logs capture the internal cause.
	require.Equal(t, "failed to load user: database password leaked", err.Error())
}

func TestNewErrorCapturesStackTraceAtConstructionSite(t *testing.T) {
	err := newSampleStackError()

	stackTrace := errorstack.Origin(err)
	require.NotEmpty(t, stackTrace)

	// The innermost frame must be the construction site, not the
	// framework-internal constructor chain.
	lines := strings.Split(stackTrace, "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	require.Contains(t, lines[0], "newSampleStackError")
	require.Contains(t, lines[1], "error_test.go")
}

func TestNewErrorWithCauseStackTracePrefersCauseOrigin(t *testing.T) {
	err := NewErrorWithCause(http.StatusInternalServerError, "failed to load record", newSampleStackCause())

	stackTrace := errorstack.Origin(err)
	require.NotEmpty(t, stackTrace)

	// The cause carries its own stack trace, which is deeper than the one
	// captured by NewErrorWithCause, so it wins as the reported origin.
	lines := strings.Split(stackTrace, "\n")
	require.Contains(t, lines[0], "newSampleStackCause")
}

func TestErrorStackTraceOnNilReceiverIsEmpty(t *testing.T) {
	require.Nil(t, (*Error)(nil).StackTrace())
}

// newSampleStackError constructs a service error inside a dedicated helper,
// so tests can assert the captured stack points at this construction site.
func newSampleStackError() *Error {
	return NewError(http.StatusConflict, "sample record missing")
}

// newSampleStackCause creates a cause error with its own embedded stack
// trace, deeper than the service error construction site.
func newSampleStackCause() error {
	return errors.New("sample cause failure")
}
