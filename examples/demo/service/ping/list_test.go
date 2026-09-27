package ping_test

import (
	"testing"

	"demo/model"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestList covers GET /api/pings, served by Lister in list.go: the route is
// public, so the request carries no session, and the answer carries the
// record count the component keeps.
func TestList(t *testing.T) {
	cli, err := client.New(testutil.BaseURL())
	require.NoError(t, err)

	rsp, err := cli.Get[model.PingRsp](t.Context(), "/api/pings")
	require.NoError(t, err)
	require.Equal(t, "pong", rsp.Msg)
	require.GreaterOrEqual(t, rsp.Records, int64(0))
}
