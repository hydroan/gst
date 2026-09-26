package testutil

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
)

// BaseURL returns the test server base address clients are constructed with.
func BaseURL() string {
	return URL("")
}

// URL returns an absolute URL of the test server for path.
func URL(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", serverPort, path)
}

// GRPCTarget returns the address the test server's gRPC listener is dialed
// at, the target a grpc.NewClient takes. Like the HTTP port, the port is
// picked per test binary, so the target can be declared as a package-level
// variable; the listener itself comes up only when the test binary
// registers gRPC services, see Run.
func GRPCTarget() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(grpcPort))
}

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
