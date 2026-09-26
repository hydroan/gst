package testutil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestURLTargetsTheTestServerPort(t *testing.T) {
	require.Equal(t, fmt.Sprintf("http://127.0.0.1:%d/api/samples", serverPort), URL("/api/samples"))
}

func TestGRPCTargetTargetsTheTestServersGRPCPort(t *testing.T) {
	require.Equal(t, fmt.Sprintf("127.0.0.1:%d", grpcPort), GRPCTarget())
}

// TestWaitForGRPCNeedsTheListener guards the wait Run makes for the gRPC
// listener: it returns once the listener accepts, which the package's own
// TestMain brought up by registering a service, and reports a port nothing
// listens on.
func TestWaitForGRPCNeedsTheListener(t *testing.T) {
	require.NoError(t, waitForGRPC(5*time.Second))

	unused, err := freeLocalPorts(1)
	require.NoError(t, err)
	saved := grpcPort
	grpcPort = unused[0]
	t.Cleanup(func() { grpcPort = saved })
	require.ErrorContains(t, waitForGRPC(200*time.Millisecond), "grpc listener on port")
}
