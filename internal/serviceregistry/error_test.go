package serviceregistry

import (
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
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

func TestNewErrorWithCauseKeepsCauseInternal(t *testing.T) {
	cause := errors.New("database password leaked")
	err := NewErrorWithCause(http.StatusInternalServerError, "failed to load user", cause)

	require.ErrorIs(t, err, cause)
	require.Equal(t, "failed to load user", err.Msg())
	require.Equal(t, "failed to load user", err.Error())
	require.NotContains(t, err.Error(), cause.Error())
}
