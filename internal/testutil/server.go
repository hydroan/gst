package testutil

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
)

// URL returns an absolute URL of the test server for path.
func URL(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", serverPort, path)
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
