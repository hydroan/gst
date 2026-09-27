package rebuild_test

import (
	"context"
	"net/http"
	"sync"
	"testing"

	// The application registers its models, services, jobs, leader work,
	// locks, interceptors and gRPC services through the init of these
	// packages, exactly as main.go imports them.
	_ "cluster/configx"
	_ "cluster/cronjob"
	_ "cluster/interceptor"
	"cluster/internal/testsupport"
	_ "cluster/leader"
	_ "cluster/lock"
	_ "cluster/middleware"
	"cluster/model"
	_ "cluster/module"
	"cluster/pb"
	"cluster/router"
	_ "cluster/service"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: config.DBMySQL,
		Redis:    true,
		Routes:   router.Init,
	})
}

// TestCreateRunsOnceAtATime proves the lock behind the action: of two
// rebuilds requested at once, one runs and the other is refused with 409
// right away, and once the first is done the next request runs again. Every
// rebuild is recorded as a run under the lock, marked ended once it ran to its
// end.
func TestCreateRunsOnceAtATime(t *testing.T) {
	cli := testsupport.Login(t).Client

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, postErr := cli.Post[model.RebuildRsp](t.Context(), "/api/rebuilds", model.RebuildReq{Seconds: 2})
			results <- postErr
		})
	}
	wg.Wait()
	close(results)

	ran, refused := 0, 0
	for err := range results {
		var refusal *client.Error
		switch {
		case err == nil:
			ran++
		case errors.As(err, &refusal) && refusal.StatusCode == http.StatusConflict:
			refused++
		default:
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, ran, "exactly one of two concurrent rebuilds runs")
	require.Equal(t, 1, refused, "the other is refused at once with 409")

	rsp, err := cli.Post[model.RebuildRsp](t.Context(), "/api/rebuilds", model.RebuildReq{Seconds: 1})
	require.NoError(t, err, "the lock is free again once the rebuild returned")
	require.Equal(t, 1, rsp.Seconds)

	runs := make([]*model.Run, 0)
	require.NoError(t, database.Database[*model.Run](context.Background()).WithQuery(&model.Run{Kind: "lock"}).List(&runs))
	require.Len(t, runs, 2, "every rebuild is recorded as one run")
	for _, run := range runs {
		require.NotNil(t, run.EndedAt, "a rebuild that ran to its end is marked ended")
	}
}

// TestCreateOverGRPCRunsOnceAtATime proves the same lock behind the
// CreateRebuild rpc: of two rebuilds requested at once one runs and the other
// is refused with AlreadyExists, the status a 409 becomes over gRPC; the runs
// then list over the ListRun rpc, every one marked ended.
func TestCreateOverGRPCRunsOnceAtATime(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	rebuilds := pb.NewRebuildServiceClient(testsupport.Dial(t))

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, callErr := rebuilds.CreateRebuild(ctx, &pb.CreateRebuildRequest{Payload: &pb.RebuildReq{Seconds: 2}})
			results <- callErr
		})
	}
	wg.Wait()
	close(results)

	ran, refused := 0, 0
	for err := range results {
		switch status.Code(err) {
		case codes.OK:
			ran++
		case codes.AlreadyExists:
			refused++
		default:
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, ran, "exactly one of two concurrent rebuilds runs")
	require.Equal(t, 1, refused, "the other is refused at once with AlreadyExists")

	runs, err := pb.NewRunServiceClient(testsupport.Dial(t)).ListRun(ctx, &pb.ListRunRequest{
		Filters: []*pb.ListRunRequest_Filter{{Field: "kind", Values: []string{"lock"}}},
		SortBy:  []string{"created_at desc"},
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, runs.GetTotal(), int64(1), "every rebuild is recorded as one run")
	require.NotNil(t, runs.GetItems()[0].GetEndedAt(), "a rebuild that ran to its end is marked ended")
	require.Equal(t, "rebuild", runs.GetItems()[0].GetName())
}
