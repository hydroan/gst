package client_test

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

func TestErrorRendersReadableText(t *testing.T) {
	err := &client.Error{StatusCode: 403, Code: 403, Msg: "permission denied", TraceID: "trace-1"}
	require.EqualError(t, err, `server rejected: status=403 code=403 msg="permission denied" trace_id=trace-1`)
}

func TestErrorSurvivesWrappingForErrorsAs(t *testing.T) {
	wrapped := errors.Wrap(&client.Error{StatusCode: 404, Msg: "not found"}, "call sample endpoint")

	var respErr *client.Error
	require.True(t, errors.As(wrapped, &respErr))
	require.Equal(t, 404, respErr.StatusCode)
	require.Equal(t, "not found", respErr.Msg)
}
