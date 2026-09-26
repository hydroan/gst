package testutil_test

import (
	"testing"

	"github.com/hydroan/gst/internal/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TestGRPCTargetReachesTheTestServersListener pins what a project's gRPC
// tests build on: the test server serves the registered services on
// GRPCTarget by the time the tests run, health included.
func TestGRPCTargetReachesTheTestServersListener(t *testing.T) {
	conn, err := grpc.NewClient(testutil.GRPCTarget(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	health, err := grpc_health_v1.NewHealthClient(conn).Check(t.Context(), &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, health.GetStatus())

	require.NoError(t, conn.Invoke(t.Context(), probeMethod, &emptypb.Empty{}, &emptypb.Empty{}))
}
