package testutil

import (
	"encoding/json"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/internal/response"
	"github.com/stretchr/testify/require"
)

// DecodeResp asserts that resp carries a successful envelope and returns the
// decoded payload.
func DecodeResp[RSP any](t *testing.T, resp *client.Envelope) RSP {
	t.Helper()

	require.NotNil(t, resp)
	require.Equal(t, response.CodeSuccess.Code(), resp.Code)
	require.Equal(t, response.CodeSuccess.Msg(), resp.Msg)
	require.NotEmpty(t, resp.TraceID)
	require.NotEmpty(t, resp.Data)

	var rsp RSP
	require.NoError(t, json.Unmarshal(resp.Data, &rsp), "response data: %s", string(resp.Data))
	return rsp
}

// RequireError asserts that err is a server-side rejection with the given
// HTTP status code and that the business message contains every msgContains
// entry. The rejection is returned by value for follow-up asserts, such as
// the business code; the value form keeps ignoring it errcheck-clean.
func RequireError(t *testing.T, err error, statusCode int, msgContains ...string) client.Error {
	t.Helper()

	require.Error(t, err)
	var respErr *client.Error
	require.True(t, errors.As(err, &respErr), "not a server rejection: %v", err)
	require.Equal(t, statusCode, respErr.StatusCode, "unexpected rejection: %v", respErr)
	for _, want := range msgContains {
		require.Contains(t, respErr.Msg, want, "unexpected rejection: %v", respErr)
	}
	return *respErr
}

// RequireDataFields asserts that the raw envelope data carries every named
// top-level JSON field, guarding serialization contracts against fields
// silently vanishing behind omitempty.
func RequireDataFields(t *testing.T, resp *client.Envelope, fields ...string) {
	t.Helper()

	require.NotNil(t, resp)
	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Data, &data), "response data: %s", string(resp.Data))
	for _, field := range fields {
		require.Contains(t, data, field, "response data: %s", string(resp.Data))
	}
}
