package tally_test

import (
	"io"
	"testing"

	"demo/model"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestSum covers GET /api/tallies, served by Sum in sum.go: a WebSocket
// connection, public, answering every amount with the total so far until the
// client closes its side.
func TestSum(t *testing.T) {
	cli, err := client.New(testutil.BaseURL())
	require.NoError(t, err)

	stream, err := cli.BidiStream[model.TallySumReq, model.TallySumRsp](t.Context(), "/api/tallies")
	require.NoError(t, err)
	defer stream.Close()
	for _, step := range []struct{ amount, total int64 }{{2, 2}, {3, 5}, {-1, 4}} {
		require.NoError(t, stream.Send(&model.TallySumReq{Amount: step.amount}))
		rsp, recvErr := stream.Recv()
		require.NoError(t, recvErr)
		require.Equal(t, step.total, rsp.Total)
	}
	require.NoError(t, stream.CloseSend())
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF, "the server closes the connection once the client has closed its side")
}
