package testutil

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
)

const fixedModuleTestPort = 8000

// serverPort is the port the test server listens on. It is picked when the
// package loads, before any package-level URL is built, so that a test can
// declare its endpoints as package-level variables.
var serverPort = mustFreeLocalPort()

// URL returns an absolute URL of the test server for path.
func URL(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", serverPort, path)
}

// listenOnFreePort configures the HTTP server to listen on the port URL
// resolves to.
func listenOnFreePort() {
	os.Setenv(config.SERVER_LISTEN, "127.0.0.1")
	os.Setenv(config.SERVER_PORT, strconv.Itoa(serverPort))
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

func mustFreeLocalPort() int {
	port, err := freeLocalPort()
	if err != nil {
		panic(err)
	}
	return port
}

func freeLocalPort() (int, error) {
	for range 10 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}

		addr, ok := l.Addr().(*net.TCPAddr)
		if !ok {
			if err := l.Close(); err != nil {
				return 0, err
			}
			return 0, errors.Newf("unexpected listener address type %T", l.Addr())
		}
		port := addr.Port
		if err := l.Close(); err != nil {
			return 0, err
		}
		if port != fixedModuleTestPort {
			return port, nil
		}
	}

	return 0, errors.Newf("failed to allocate a non-%d local port", fixedModuleTestPort)
}
