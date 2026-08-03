package testutil

import (
	"encoding/json"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/internal/response"
	"github.com/stretchr/testify/require"
)

// RequireResp asserts that resp carries a successful envelope and hands the
// decoded payload to checkFn.
func RequireResp[RSP any](t *testing.T, resp *client.Resp, checkFn func(t *testing.T, rsp RSP)) {
	t.Helper()

	require.NotNil(t, resp)
	require.Equal(t, response.CodeSuccess.Code(), resp.Code)
	require.Equal(t, response.CodeSuccess.Msg(), resp.Msg)
	require.NotEmpty(t, resp.TraceID)
	require.NotEmpty(t, resp.Data)

	var rsp RSP
	require.NoError(t, json.Unmarshal(resp.Data, &rsp), "response data: %s", string(resp.Data))
	if checkFn != nil {
		checkFn(t, rsp)
	}
}

// RequireError asserts that err is a server-side rejection with the given
// HTTP status code and that the business message contains every msgContains
// entry.
func RequireError(t *testing.T, err error, statusCode int, msgContains ...string) {
	t.Helper()

	require.Error(t, err)
	var respErr *client.Error
	require.True(t, errors.As(err, &respErr), "not a server rejection: %v", err)
	require.Equal(t, statusCode, respErr.StatusCode, "unexpected rejection: %v", respErr)
	for _, want := range msgContains {
		require.Contains(t, respErr.Msg, want, "unexpected rejection: %v", respErr)
	}
}
