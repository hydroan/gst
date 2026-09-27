package cached_test

import (
	"testing"

	// The application registers its models, services, jobs, leader work,
	// locks, interceptors and gRPC services through the init of these
	// packages, exactly as main.go imports them.
	_ "cluster/component"
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

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: config.DBMySQL,
		Redis:    true,
		Kafka:    true,
		Routes:   router.Init,
	})
}

// TestCreateCachesTheEntry proves the write path of the replicated cache, over
// HTTP and over the CreateCached rpc alike: the entry lands in the store of
// the replica that answered, which is what its peers then catch up with. The
// propagation between replicas is what the deployment scenarios in the README
// check; one process can only show that the entry was cached at all.
func TestCreateCachesTheEntry(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	t.Run("over HTTP", func(t *testing.T) {
		cli := testsupport.Login(t).Client

		rsp, err := cli.Post[model.CachedCreateRsp](ctx, "/api/caches", model.CachedReq{Key: "created-key", Value: "created-value"})
		require.NoError(t, err)
		require.Equal(t, "created-key", rsp.Key)
		require.NotEmpty(t, rsp.Replica, "the reply names the replica that cached the entry")

		read, err := cli.Get[model.CachedGetRsp](ctx, "/api/caches/created-key")
		require.NoError(t, err)
		require.True(t, read.Found, "the entry is in the store of the replica that wrote it")
		require.Equal(t, "created-value", read.Value)
	})

	t.Run("over gRPC", func(t *testing.T) {
		caches := pb.NewCachedServiceClient(testsupport.Dial(t))

		rsp, err := caches.CreateCached(ctx, &pb.CreateCachedRequest{Payload: &pb.CachedReq{Key: "created-over-grpc", Value: "created-value"}})
		require.NoError(t, err)
		require.Equal(t, "created-over-grpc", rsp.GetResult().GetKey())
		require.NotEmpty(t, rsp.GetResult().GetReplica())

		read, err := caches.GetCached(ctx, &pb.GetCachedRequest{Id: "created-over-grpc"})
		require.NoError(t, err)
		require.True(t, read.GetResult().GetFound(), "the entry is in the store of the replica that wrote it")
		require.Equal(t, "created-value", read.GetResult().GetValue())
	})
}
