package counterstep_test

import (
	"context"
	"testing"
	"time"

	"cluster/internal/testsupport"
	"cluster/model"
	"cluster/pb"

	"github.com/hydroan/gst/database"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/testutil"
	"github.com/hydroan/gst/testutil/iamtest"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestWatch covers the Stream action on /api/counter_steps/watch, served by
// Watch in watch.go: the numbers after the one asked for, those already in
// the table first and then one written while the stream is open, and the
// stream ending when the client hangs up. Over gRPC it is the
// WatchCounterStep rpc, the session traveling as a bearer token; over HTTP it
// is a WebSocket connection, the request its first message and the session
// traveling in the cookie the login set. The leader work of the test process
// writes numbers of its own meanwhile, which the stream carries too; the test
// looks for its own.
func TestWatch(t *testing.T) {
	t.Run("over gRPC", func(t *testing.T) {
		ctx := gstgrpc.WithBearer(t.Context(), iamtest.GRPCSession(t, testsupport.Root))
		base := time.Now().UnixNano() / int64(time.Millisecond) * 1000
		writeStep(t, base+1)
		writeStep(t, base+2)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		steps := pb.NewCounterStepServiceClient(testutil.DialGRPC(t))
		stream, err := steps.WatchCounterStep(ctx, &pb.WatchCounterStepRequest{Payload: &pb.CounterStepWatchReq{After: base}})
		require.NoError(t, err)
		requireWatch(t, base, func() (int64, error) {
			rsp, recvErr := stream.Recv()
			return rsp.GetResult().GetSeq(), recvErr
		})

		cancel()
		_, err = stream.Recv()
		require.Equal(t, codes.Canceled, status.Code(err), "hanging up ends the stream")
	})

	t.Run("over WebSocket", func(t *testing.T) {
		cli := iamtest.Login(t, testsupport.Root).Client
		base := time.Now().UnixNano() / int64(time.Millisecond) * 1000
		writeStep(t, base+1)
		writeStep(t, base+2)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		stream, err := cli.ServerStream[model.CounterStep](ctx, "/api/counter_steps/watch", &model.CounterStepWatchReq{After: base})
		require.NoError(t, err)
		defer stream.Close()
		requireWatch(t, base, func() (int64, error) {
			step, recvErr := stream.Recv()
			if recvErr != nil {
				return 0, recvErr
			}
			return step.Seq, nil
		})

		cancel()
		_, err = stream.Recv()
		require.ErrorIs(t, err, context.Canceled, "hanging up ends the stream")
	})
}

// TestList covers the ListCounterStep rpc: the numbers of the counter,
// filtered and sorted as the HTTP list is.
func TestList(t *testing.T) {
	ctx := gstgrpc.WithBearer(t.Context(), iamtest.GRPCSession(t, testsupport.Root))
	base := time.Now().UnixNano()/int64(time.Millisecond)*1000 + 500
	for _, seq := range []int64{base + 1, base + 2} {
		require.NoError(t, database.Database[*model.CounterStep](context.Background()).Create(&model.CounterStep{Seq: seq, Tenure: "list-test", Replica: "list-test"}))
	}

	steps := pb.NewCounterStepServiceClient(testutil.DialGRPC(t))
	rsp, err := steps.ListCounterStep(ctx, &pb.ListCounterStepRequest{
		Filters: []*pb.ListCounterStepRequest_Filter{{Field: "tenure", Values: []string{"list-test"}}},
		SortBy:  []string{"seq desc"},
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, rsp.GetTotal())
	require.Equal(t, base+2, rsp.GetItems()[0].GetSeq())
	require.Equal(t, "list-test", rsp.GetItems()[0].GetReplica())
}

// writeStep writes a number of the counter the way the leader work does.
func writeStep(t *testing.T, seq int64) {
	t.Helper()

	require.NoError(t, database.Database[*model.CounterStep](context.Background()).Create(&model.CounterStep{Seq: seq, Tenure: "watch-test", Replica: "watch-test"}))
}

// requireWatch plays out a watch opened after base, next returning the number
// its stream carries next: the two numbers written before it opened arrive,
// then one written while it is open.
func requireWatch(t *testing.T, base int64, next func() (int64, error)) {
	t.Helper()

	seen := make(map[int64]bool)
	receiveUntil := func(seq int64) {
		for !seen[seq] {
			got, err := next()
			require.NoError(t, err)
			require.Greater(t, got, base, "only the numbers after the one asked for are streamed")
			seen[got] = true
		}
	}
	receiveUntil(base + 2)
	require.True(t, seen[base+1], "the numbers already written come first")

	writeStep(t, base+100)
	receiveUntil(base + 100)
}
