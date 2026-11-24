package gst_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/errorstack"
	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

func TestNewError(t *testing.T) {
	err := gst.NewError(http.StatusBadRequest, "invalid input")

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, err.Status())
	require.Equal(t, "invalid input", err.Msg())
	require.Equal(t, "invalid input", err.Error())
}

func TestNewErrorNormalizesInvalidStatus(t *testing.T) {
	for _, status := range []int{0, http.StatusOK, http.StatusFound, 99, 600} {
		err := gst.NewError(status, "should not leak")

		require.Equal(t, http.StatusInternalServerError, err.Status())
		require.Equal(t, http.StatusText(http.StatusInternalServerError), err.Msg())
	}
}

func TestNewErrorUsesHTTPStatusTextWhenMessageIsEmpty(t *testing.T) {
	err := gst.NewError(http.StatusNotFound, "")

	require.Equal(t, http.StatusNotFound, err.Status())
	require.Equal(t, http.StatusText(http.StatusNotFound), err.Msg())
}

func TestNewErrorWithCauseIncludesCauseInErrorButNotMsg(t *testing.T) {
	cause := errors.New("database password leaked")
	err := gst.NewErrorWithCause(http.StatusInternalServerError, "failed to load user", cause)

	require.ErrorIs(t, err, cause)
	// Msg stays client-safe: the response envelope renders Msg, never Error.
	require.Equal(t, "failed to load user", err.Msg())
	require.NotContains(t, err.Msg(), cause.Error())
	// Error reports the full chain so logs capture the internal cause.
	require.Equal(t, "failed to load user: database password leaked", err.Error())
}

func TestNewErrorCapturesStackTraceAtConstructionSite(t *testing.T) {
	err := gst.NewError(http.StatusConflict, "sample record missing")

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
	err := gst.NewErrorWithCause(http.StatusInternalServerError, "failed to load record", cause)

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
	require.Nil(t, (*gst.Error)(nil).StackTrace())
}

// TestErrorOnANilReceiverAnswersTheServerFailure pins what a typed nil
// *Error answers, the error a `var e *Error; return e` hands its caller: the
// default status, 500, and FailureMsg as its message and its text alike, so
// the envelope and the gRPC status word it as they word every other failure
// of the server's own.
func TestErrorOnANilReceiverAnswersTheServerFailure(t *testing.T) {
	var err *gst.Error
	require.Equal(t, http.StatusInternalServerError, err.Status())
	require.Equal(t, gst.FailureMsg, err.Msg())
	require.Equal(t, gst.FailureMsg, err.Error())
}

// TestNewInvalidFieldsJoinsTheViolations pins the error of a request whose
// fields the validator refused: 400, a message joining the description of
// each violation with a semicolon, the violations readable for the gRPC
// details, and the validator's error as the cause.
func TestNewInvalidFieldsJoinsTheViolations(t *testing.T) {
	cause := errors.New("validation failed")
	violations := []gst.FieldViolation{
		{Field: "name", Description: "name is a required field"},
		{Field: "address.city", Description: "address.city is a required field"},
	}

	err := gst.NewInvalidFields(violations, cause)

	require.Equal(t, http.StatusBadRequest, err.Status())
	require.Equal(t, "name is a required field; address.city is a required field", err.Msg())
	require.Equal(t, violations, gst.FieldViolations(err))
	require.ErrorIs(t, err, cause)
	require.Empty(t, gst.FieldViolations(gst.NewError(http.StatusBadRequest, "plain")))
}

// TestNewInvalidValueNamesTheField pins the error of a request whose field
// carries a value it cannot hold: 400, the message naming the field by its
// JSON key path, one violation carrying the field and the description for
// the gRPC details, and the decoder's error as the cause, reported by Error
// and kept out of the message.
func TestNewInvalidValueNamesTheField(t *testing.T) {
	cause := errors.New("300 does not fit int8")

	err := gst.NewInvalidValue("items[1].rank", "300 is out of range", cause)

	require.Equal(t, http.StatusBadRequest, err.Status())
	require.Equal(t, "invalid value for field 'items[1].rank'", err.Msg())
	require.Equal(t, []gst.FieldViolation{{Field: "items[1].rank", Description: "300 is out of range"}}, gst.FieldViolations(err))
	require.ErrorIs(t, err, cause)
	require.Equal(t, "invalid value for field 'items[1].rank': 300 does not fit int8", err.Error())
	require.Equal(t, "invalid value for field 'rank'", gst.NewInvalidValue("rank", "300 is out of range", nil).Error())
}

// TestServerFailureTellsTheServersFailuresFromTheClients pins the line the
// framework's own records of a failure turn on: an error carrying no service
// error, or one answering 500 or above, is the server's own failure; a
// service error answering below 500, wherever it sits in the wrap chain, is
// one the client brought on itself.
func TestServerFailureTellsTheServersFailuresFromTheClients(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"an error carrying no service error", errors.New("dial tcp: connection refused"), true},
		{"a typed nil service error", (*gst.Error)(nil), true},
		{"a 500", gst.NewError(http.StatusInternalServerError, "broken"), true},
		{"a 503", gst.NewError(http.StatusServiceUnavailable, "later"), true},
		{"a 400", gst.NewError(http.StatusBadRequest, "bad request"), false},
		{"a 404", gst.NewError(http.StatusNotFound, "no such record"), false},
		{"a 499", gst.NewError(gst.StatusClientClosedRequest, "gone"), false},
		{"a wrapped 403", errors.Wrap(gst.NewError(http.StatusForbidden, "not yours"), "create sample"), false},
		{"a 409 carrying a cause", gst.NewErrorWithCause(http.StatusConflict, "taken", errors.New("duplicate entry")), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, gst.ServerFailure(tc.err))
		})
	}
}

// TestFailureLevelFollowsTheStatus pins the level a failure is logged at,
// from the status it is answered with: error for the server's own failure,
// an error carrying no service error or one answering 500 or above; warn for
// a permission refused, a request that did not arrive in time, a
// precondition not met and a rate exceeded; info for every other failure the
// client brought on itself, a conflict among them whatever caused it.
func TestFailureLevelFollowsTheStatus(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want slog.Level
	}{
		{"400", gst.NewError(http.StatusBadRequest, "bad request"), slog.LevelInfo},
		{"401", gst.NewError(http.StatusUnauthorized, "who are you"), slog.LevelInfo},
		{"403", gst.NewError(http.StatusForbidden, "not yours"), slog.LevelWarn},
		{"404", gst.NewError(http.StatusNotFound, "no such record"), slog.LevelInfo},
		{"408", gst.NewError(http.StatusRequestTimeout, "too slow"), slog.LevelWarn},
		{"409", gst.NewError(http.StatusConflict, "taken"), slog.LevelInfo},
		{"409 carrying a cause", gst.NewErrorWithCause(http.StatusConflict, "reload", errors.New("the record was modified by another operation")), slog.LevelInfo},
		{"412", gst.NewError(http.StatusPreconditionFailed, "not yet"), slog.LevelWarn},
		{"422", gst.NewError(http.StatusUnprocessableEntity, "cannot"), slog.LevelInfo},
		{"429", gst.NewError(http.StatusTooManyRequests, "slow down"), slog.LevelWarn},
		{"499", gst.NewError(gst.StatusClientClosedRequest, "gone"), slog.LevelInfo},
		{"500", gst.NewError(http.StatusInternalServerError, "broken"), slog.LevelError},
		{"501", gst.NewError(http.StatusNotImplemented, "not here"), slog.LevelError},
		{"503", gst.NewError(http.StatusServiceUnavailable, "later"), slog.LevelError},
		{"504", gst.NewError(http.StatusGatewayTimeout, "upstream"), slog.LevelError},
		{"a wrapped service error", errors.Wrap(gst.NewError(http.StatusForbidden, "not yours"), "create sample"), slog.LevelWarn},
		{"a typed nil service error", (*gst.Error)(nil), slog.LevelError},
		{"any other error", errors.New("dial tcp: connection refused"), slog.LevelError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, gst.FailureLevel(tc.err))
		})
	}
}
