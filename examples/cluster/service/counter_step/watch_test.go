package counterstep_test

import (
	"context"
	"testing"
	"time"

	"cluster/internal/testsupport"
	"cluster/model"
	"cluster/pb"

	"github.com/hydroan/gst/database"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestWatch covers the WatchCounterStep rpc, served by Watch in watch.go: the
// numbers after the one asked for, those already in the table first and then
// one written while the stream is open, and the stream ending when the client
// hangs up. The leader work of the test process writes numbers of its own
// meanwhile, which the stream carries too; the test looks for its own.
func TestWatch(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	base := time.Now().UnixNano() / int64(time.Millisecond) * 1000
	write := func(seq int64) {
		require.NoError(t, database.Database[*model.CounterStep](context.Background()).Create(&model.CounterStep{Seq: seq, Tenure: "watch-test", Replica: "watch-test"}))
	}
	write(base + 1)
	write(base + 2)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	steps := pb.NewCounterStepServiceClient(testsupport.Dial(t))
	stream, err := steps.WatchCounterStep(ctx, &pb.WatchCounterStepRequest{Payload: &pb.CounterStepWatchReq{After: base}})
	require.NoError(t, err)

	seen := make(map[int64]bool)
	receiveUntil := func(seq int64) {
		for !seen[seq] {
			rsp, recvErr := stream.Recv()
			require.NoError(t, recvErr)
			require.Greater(t, rsp.GetResult().GetSeq(), base, "only the numbers after the one asked for are streamed")
			seen[rsp.GetResult().GetSeq()] = true
		}
	}
	receiveUntil(base + 2)
	require.True(t, seen[base+1], "the numbers already written come first")

	write(base + 100)
	receiveUntil(base + 100)

	cancel()
	_, err = stream.Recv()
	require.Equal(t, codes.Canceled, status.Code(err), "hanging up ends the stream")
}

// TestList covers the ListCounterStep rpc: the numbers of the counter,
// filtered and sorted as the HTTP list is.
func TestList(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	base := time.Now().UnixNano()/int64(time.Millisecond)*1000 + 500
	for _, seq := range []int64{base + 1, base + 2} {
		require.NoError(t, database.Database[*model.CounterStep](context.Background()).Create(&model.CounterStep{Seq: seq, Tenure: "list-test", Replica: "list-test"}))
	}

	steps := pb.NewCounterStepServiceClient(testsupport.Dial(t))
	rsp, err := steps.ListCounterStep(ctx, &pb.ListCounterStepRequest{
		Filters: []*pb.ListCounterStepRequest_Filter{{Field: "tenure", Values: []string{"list-test"}}},
		SortBy:  []string{"seq desc"},
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, rsp.GetTotal())
	require.Equal(t, base+2, rsp.GetItems()[0].GetSeq())
	require.Equal(t, "list-test", rsp.GetItems()[0].GetReplica())
}
