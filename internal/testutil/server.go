package testutil

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
)

const fixedModuleTestPort = 8000

var redisNamespaceSeq atomic.Uint64

// SetupRandomServerPort configures the HTTP server to listen on a local ephemeral port.
func SetupRandomServerPort() int {
	port, err := freeLocalPort()
	if err != nil {
		panic(err)
	}

	os.Setenv(config.SERVER_LISTEN, "127.0.0.1")
	os.Setenv(config.SERVER_PORT, strconv.Itoa(port))
	return port
}

// SetupRandomRedisNamespace configures Redis to use a namespace unique to this test process.
func SetupRandomRedisNamespace() string {
	seq := redisNamespaceSeq.Add(1)
	namespace := fmt.Sprintf("gst-test:%d:%d", os.Getpid(), seq)
	os.Setenv(config.REDIS_NAMESPACE, namespace)
	return namespace
}

// URL returns an absolute URL for the configured test server port.
func URL(port int, path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
}

// MustWaitForServer waits until the test server responds to health checks.
func MustWaitForServer(port int) {
	if err := WaitForServer(port, 10*time.Second); err != nil {
		panic(err)
	}
}

// WaitForServer waits until the test server responds to health checks.
func WaitForServer(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	cli := &http.Client{Timeout: 200 * time.Millisecond}
	url := URL(port, "/-/healthz")
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
	return errors.Wrapf(lastErr, "server on port %d did not become ready", port)
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
