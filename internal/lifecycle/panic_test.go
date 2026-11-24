package lifecycle_test

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/stretchr/testify/require"
)

// TestPanicErrorKeepsAnErrorAndWordsAnyOtherValue pins what a recovered panic
// is reported as: a panic value that is an error stays that error, any other
// value becomes an error worded the way the value prints, and either carries
// a stack.
func TestPanicErrorKeepsAnErrorAndWordsAnyOtherValue(t *testing.T) {
	cause := errors.New("sample failure")
	require.ErrorIs(t, lifecycle.PanicError(cause), cause)

	worded := lifecycle.PanicError("sample panic")
	require.EqualError(t, worded, "sample panic")
	require.NotNil(t, errors.GetReportableStackTrace(worded))
	require.EqualError(t, lifecycle.PanicError(42), "42")
}
