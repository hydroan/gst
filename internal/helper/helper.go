package helper

import (
	"encoding/json"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/internal/response"
	"github.com/stretchr/testify/require"
)

func TestResp[RSP any](t *testing.T, resp *client.Resp, checkFn func(t *testing.T, rsp RSP)) {
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
