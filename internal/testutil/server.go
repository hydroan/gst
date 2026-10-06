package testutil

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/grpcserver"
)

// BaseURL returns the test server base address clients are constructed with.
func BaseURL() string {
	return URL("")
}

// URL returns an absolute URL of the test server for path.
func URL(path string) string {
	return fmt.Sprintf("http://%s%s", net.JoinHostPort(loopbackHost, strconv.Itoa(serverPort)), path)
}

// GRPCTarget returns the address the test server's gRPC listener is dialed
// at, the target a grpc.NewClient takes. Like the HTTP port, the port is
// picked per test binary, so the target can be declared as a package-level
// variable; the listener itself comes up only when the test binary
// registers gRPC services, see Run, and a test binary that registered none
// is told so instead of dialing a port nothing listens on: GRPCTarget
// panics naming the import that registers them, the project's pb package
// in the test file declaring TestMain, the way main.go imports it.
func GRPCTarget() string {
	if !grpcServed() {
		panic(`testutil: the test binary registered no gRPC service, so the test server serves no gRPC; import the project's pb package in the test file declaring TestMain the way main.go does, _ "<module>/pb"`)
	}
	return net.JoinHostPort(loopbackHost, strconv.Itoa(grpcPort))
}

// grpcServed reports whether the test binary registered gRPC services; a
// variable for the test of GRPCTarget to run with none.
var grpcServed = grpcserver.HasServices

// mustWaitForGRPC waits until the test server's gRPC listener accepts
// connections.
func mustWaitForGRPC() {
	if err := waitForGRPC(10 * time.Second); err != nil {
		panic(err)
	}
}

// waitForGRPC waits until the gRPC listener accepts a connection. The server
// binds its listener right before it serves, with the health service already
// registered, so a connection the kernel accepts means the calls that follow
// are answered.
func waitForGRPC(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", GRPCTarget(), 200*time.Millisecond)
		if err == nil {
			return conn.Close()
		}
		lastErr = err

		time.Sleep(20 * time.Millisecond)
	}

	if lastErr == nil {
		lastErr = errors.New("listener did not respond before timeout")
	}
	return errors.Wrapf(lastErr, "grpc listener on port %d did not become ready", grpcPort)
}

// mustWaitForServer waits until the test server responds to health checks.
func mustWaitForServer() {
	if err := waitForServer(10 * time.Second); err != nil {
		panic(err)
	}
}

// waitForServer waits until the test server responds to health checks.
func waitForServer(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	cli := &http.Client{Timeout: 200 * time.Millisecond}
	url := URL("/-/healthz")
	var lastErr error

	for time.Now().Before(deadline) {
		resp, err := cli.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < http.StatusInternalServerError {
				return nil
			}
			lastErr = errors.Newf("health check returned status %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		time.Sleep(20 * time.Millisecond)
	}

	if lastErr == nil {
		lastErr = errors.New("server did not respond before timeout")
	}
	return errors.Wrapf(lastErr, "server on port %d did not become ready", serverPort)
}
