package testcontainer

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
)

// containerEndpoint returns the host and the published port c maps port to.
func containerEndpoint(ctx context.Context, c testcontainers.Container, port string) (string, uint, error) {
	host, err := c.Host(ctx)
	if err != nil {
		return "", 0, errors.Wrapf(err, "failed to resolve container host for port %s", port)
	}
	mapped, err := c.MappedPort(ctx, port)
	if err != nil {
		return "", 0, errors.Wrapf(err, "failed to resolve mapped port for port %s", port)
	}
	return host, uint(mapped.Num()), nil
}

// reportServiceReady prints where a prepared service lives. Container logging
// is muted, see muteContainerLog, so this is the only line a failing test
// leaves behind to reach the service by hand.
func reportServiceReady(name, target string) {
	fmt.Fprintf(os.Stdout, "test %s ready: %s\n", name, target)
}

var muteContainerLogOnce sync.Once

// muteContainerLog silences the logging testcontainers does on its own. Its
// default logger writes to stderr as soon as the test binary runs with -v,
// which buries the output of the test itself under image pull, reaper and
// container lifecycle noise. Each setup function reports the one line that
// matters instead, see reportServiceReady.
func muteContainerLog() {
	muteContainerLogOnce.Do(func() {
		tclog.SetDefault(log.New(io.Discard, "", 0))
	})
}
