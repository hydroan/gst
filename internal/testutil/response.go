package testutil

import (
	"encoding/json"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/internal/response"
	"github.com/stretchr/testify/require"
)

// ListResponse is the envelope a list endpoint answers with, for tests that
// assert on the items and the total.
type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

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
