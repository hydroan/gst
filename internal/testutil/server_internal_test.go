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

// TestGRPCTargetPanicsWhenNoServiceIsRegistered pins what a test binary
// that never imported the project's pb package meets instead of a refused
// connection: GRPCTarget panics naming the import to add.
func TestGRPCTargetPanicsWhenNoServiceIsRegistered(t *testing.T) {
	served := grpcServed
	grpcServed = func() bool { return false }
	t.Cleanup(func() { grpcServed = served })

	require.PanicsWithValue(t, `testutil: the test binary registered no gRPC service, so the test server serves no gRPC; import the project's pb package in the test file declaring TestMain the way main.go does, _ "<module>/pb"`, func() { GRPCTarget() })
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
