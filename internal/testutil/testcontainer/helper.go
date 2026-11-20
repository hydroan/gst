package testcontainer

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
)

// endpoint returns the host and the published port c maps port to.
func endpoint(ctx context.Context, c testcontainers.Container, port string) (string, uint, error) {
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

// readyTimeout is how long a container started here is given to report
// ready. The modules give theirs a minute, some steps of it ten seconds, and
// a first start outlasts that on a machine busy with the rest of a test run:
// a mysql server ready in ten seconds on an idle machine has taken close to
// three minutes beside the checks and the other tests.
const readyTimeout = 5 * time.Minute

// patientStart returns the option that gives a container readyTimeout to
// report ready, whatever its module waits for: once the request is complete,
// before the container is created, the deadline of its wait and the startup
// timeout of every strategy in it are set to readyTimeout (see
// extendReadyWait). testcontainers has no option doing it:
// WithWaitStrategyAndDeadline replaces the strategies, which are the
// module's to know; a module may add its strategies after the caller's
// options have run; and MultiStrategy.WithStartupTimeoutDefault only ever
// shortens a wait, each strategy applying its own default of a minute.
func patientStart() testcontainers.CustomizeRequestOption {
	return testcontainers.WithAdditionalLifecycleHooks(testcontainers.ContainerLifecycleHooks{
		PreCreates: []testcontainers.ContainerRequestHook{
			func(_ context.Context, req testcontainers.ContainerRequest) error {
				return extendReadyWait(req.WaitingFor)
			},
		},
	})
}

// extendReadyWait sets to readyTimeout the time strategy waits for a
// container: the deadline of a wait made of several strategies and the
// startup timeout of each. A strategy of a kind it does not know is an
// error, so a module that starts waiting some other way is noticed here
// rather than by a start that times out after a minute again.
func extendReadyWait(strategy wait.Strategy) error {
	switch s := strategy.(type) {
	case nil:
	case *wait.MultiStrategy:
		s.WithDeadline(readyTimeout)
		for _, inner := range s.Strategies {
			if err := extendReadyWait(inner); err != nil {
				return err
			}
		}
	case *wait.LogStrategy:
		s.WithStartupTimeout(readyTimeout)
	case *wait.HTTPStrategy:
		s.WithStartupTimeout(readyTimeout)
	case *wait.HostPortStrategy:
		s.WithStartupTimeout(readyTimeout)
	default:
		return errors.Newf("wait strategy %T: no startup timeout known to extend", strategy)
	}
	return nil
}
