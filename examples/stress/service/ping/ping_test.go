package ping_test

import (
	"testing"

	"stress/model"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestPingAnswersWithoutASession covers GET /api/ping, served by Ping in
// ping.go: the route is Public(), so a client carrying no session gets pong.
func TestPingAnswersWithoutASession(t *testing.T) {
	cli, err := client.New(testutil.BaseURL())
	require.NoError(t, err)

	rsp, err := cli.Get[model.PingRsp](t.Context(), "/api/ping")
	require.NoError(t, err)
	require.Equal(t, "pong", rsp.Msg)
}
