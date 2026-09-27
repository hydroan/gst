package stepdown_test

import (
	"testing"

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

// TestCreateAsksTheLeaderWorkToReturn proves the request reaches the replica
// that took it and names it, over HTTP and over the CreateStepDown rpc alike.
// Whether that replica held the leadership is what asked reports; the
// deployment scenarios in the README send it to the one that does.
func TestCreateAsksTheLeaderWorkToReturn(t *testing.T) {
	ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
	t.Run("over HTTP", func(t *testing.T) {
		cli := testsupport.Login(t).Client

		rsp, err := cli.Post[model.StepDownRsp](ctx, "/api/step-downs", nil)
		require.NoError(t, err)
		require.NotEmpty(t, rsp.Replica, "the reply names the replica that took the request")
	})

	t.Run("over gRPC", func(t *testing.T) {
		rsp, err := pb.NewStepDownServiceClient(testsupport.Dial(t)).CreateStepDown(ctx, &pb.CreateStepDownRequest{})
		require.NoError(t, err)
		require.NotEmpty(t, rsp.GetResult().GetReplica(), "the reply names the replica that took the request")
	})
}
