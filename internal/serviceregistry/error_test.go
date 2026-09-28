package serviceregistry_test

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/errorstack"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/stretchr/testify/require"
)

func TestNewError(t *testing.T) {
	err := serviceregistry.NewError(http.StatusBadRequest, "invalid input")

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, err.Status())
	require.Equal(t, "invalid input", err.Msg())
	require.Equal(t, "invalid input", err.Error())
}

func TestNewErrorNormalizesInvalidStatus(t *testing.T) {
	for _, status := range []int{0, http.StatusOK, http.StatusFound, 99, 600} {
		err := serviceregistry.NewError(status, "should not leak")

		require.Equal(t, http.StatusInternalServerError, err.Status())
		require.Equal(t, http.StatusText(http.StatusInternalServerError), err.Msg())
	}
}

func TestNewErrorUsesHTTPStatusTextWhenMessageIsEmpty(t *testing.T) {
	err := serviceregistry.NewError(http.StatusNotFound, "")

	require.Equal(t, http.StatusNotFound, err.Status())
	require.Equal(t, http.StatusText(http.StatusNotFound), err.Msg())
}

func TestNewErrorWithCauseIncludesCauseInErrorButNotMsg(t *testing.T) {
	cause := errors.New("database password leaked")
	err := serviceregistry.NewErrorWithCause(http.StatusInternalServerError, "failed to load user", cause)

	require.ErrorIs(t, err, cause)
	// Msg stays client-safe: the response envelope renders Msg, never Error.
	require.Equal(t, "failed to load user", err.Msg())
	require.NotContains(t, err.Msg(), cause.Error())
	// Error reports the full chain so logs capture the internal cause.
	require.Equal(t, "failed to load user: database password leaked", err.Error())
}

func TestNewErrorCapturesStackTraceAtConstructionSite(t *testing.T) {
	err := serviceregistry.NewError(http.StatusConflict, "sample record missing")

	stackTrace := errorstack.Origin(err)
	require.NotEmpty(t, stackTrace)

	// The innermost frame must be the construction site, this test, not the
	// framework-internal constructor chain.
	lines := strings.Split(stackTrace, "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	require.Contains(t, lines[0], "TestNewErrorCapturesStackTraceAtConstructionSite")
	require.Contains(t, lines[1], "error_test.go")
}

func TestNewErrorWithCauseStackTracePrefersCauseOrigin(t *testing.T) {
	_, _, line, ok := runtime.Caller(0)
	require.True(t, ok)
	cause := errors.New("sample cause failure") // two lines below the lookup
	err := serviceregistry.NewErrorWithCause(http.StatusInternalServerError, "failed to load record", cause)

	stackTrace := errorstack.Origin(err)
	require.NotEmpty(t, stackTrace)

	// The cause carries its own stack trace, the innermost in the chain, so
	// its construction site is the reported origin rather than the one
	// NewErrorWithCause captured on the line after it.
	lines := strings.Split(stackTrace, "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	require.Contains(t, lines[0], "TestNewErrorWithCauseStackTracePrefersCauseOrigin")
	require.True(t, strings.HasSuffix(lines[1], fmt.Sprintf("error_test.go:%d", line+2)), lines[1])
}

func TestErrorStackTraceOnNilReceiverIsEmpty(t *testing.T) {
	require.Nil(t, (*serviceregistry.Error)(nil).StackTrace())
}
