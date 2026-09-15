package bootstrap

import (
	"context"
	"net"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/router"
	"github.com/stretchr/testify/require"
)

// TestMain gives the bootstrapped process a scratch log directory that
// outlives the tests sharing it, and removes it once they are done.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gst_bootstrap_test_")
	if err != nil {
		panic(err)
	}
	bootstrapLogDir = dir

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// TestTeardownStopsTheLogWritersLast proves the cleanups Bootstrap registers
// unwind with the log writers going last and the temp directory right before
// them: every other cleanup logs what it did, and a line written after the
// writers stopped never reaches its file.
func TestTeardownStopsTheLogWritersLast(t *testing.T) {
	bootstrapProcess(t)

	var teardown []string
	for _, cleanup := range slices.Backward(cleanups) {
		teardown = append(teardown, runtime.FuncForPC(reflect.ValueOf(cleanup).Pointer()).Name())
	}
	require.Equal(t,
		[]string{"github.com/hydroan/gst/config.Clean", "github.com/hydroan/gst/logger/zap.Clean"},
		teardown[len(teardown)-2:],
		"the temp directory and then the log writers must be the last things torn down")
}

// TestUnlinkedProvidersAreTheEnabledOnesTheBinaryLacks proves the warning
// bootstrap logs names exactly the enabled providers no package linked.
func TestUnlinkedProvidersAreTheEnabledOnesTheBinaryLacks(t *testing.T) {
	require.Equal(t, []string{"nats"}, unlinkedProviders([]string{"kafka", "nats"}, []string{"kafka", "mongo"}))
	require.Empty(t, unlinkedProviders([]string{"kafka"}, []string{"kafka"}))
	require.Empty(t, unlinkedProviders(nil, nil))
}

// TestRunDrainsBeforeTeardownWhenAListenerFails proves the order Run keeps
// at both ends. Starting: the routes-ready hooks fire before the components
// start, so what a hook seeds is there for a component's first round.
// Stopping: a listener failure stops the process the way a signal does —
// readiness goes down and the process context is canceled while the
// listener still answers, the drain window passes, and only then does
// teardown begin — with the failure returned.
func TestRunDrainsBeforeTeardownWhenAListenerFails(t *testing.T) {
	// Run is single-shot: it seals the component stage and unwinds the
	// cleanup stack, so a process can go through it once.
	if runCovered {
		t.Skip("Run ran once in this process already; the first run of this test covered it")
	}
	runCovered = true
	bootstrapProcess(t)
	original := config.App.Server.ShutdownDelay
	config.App.Server.ShutdownDelay = 2 * time.Second
	t.Cleanup(func() { config.App.Server.ShutdownDelay = original })

	var (
		orderMu sync.Mutex
		order   []string
	)
	note := func(step string) {
		orderMu.Lock()
		defer orderMu.Unlock()
		order = append(order, step)
	}
	router.OnRoutesReady(func(context.Context, map[string][]string) error {
		note("routes-ready hook")
		return nil
	})
	lifecycle.Register(lifecycle.Component{
		Name:  "sample-order-probe",
		Stage: lifecycle.StageComponent,
		Start: func(context.Context) error {
			note("component start")
			return nil
		},
	})

	errListener := errors.New("sample listener failure")
	fail := make(chan struct{})
	startup.RegisterGo(func() error {
		<-fail
		return errListener
	})

	done := make(chan error, 1)
	go func() { done <- Run() }()
	require.Eventually(t, func() bool { return readyz() == http.StatusOK },
		10*time.Second, 20*time.Millisecond, "the server never came up")

	orderMu.Lock()
	started := slices.Clone(order)
	orderMu.Unlock()
	require.Equal(t, []string{"routes-ready hook", "component start"}, started,
		"the hooks must have seeded before the components started")

	close(fail)
	require.Eventually(t, func() bool {
		return readyz() == http.StatusServiceUnavailable && processCtx.Err() != nil
	}, time.Second, 10*time.Millisecond,
		"readiness must go down and the process context must be canceled while the listener still answers")

	select {
	case err := <-done:
		require.ErrorIs(t, err, errListener)
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return once the drain window passed")
	}
}

// TestAwaitShutdownReturnsWhatEndedTheWait proves Run ends for each of the
// three reasons it may — with nothing to report for a signal, and the failure
// for a listener's or a component's — so a component failing ends the process
// the way a listener failing does.
func TestAwaitShutdownReturnsWhatEndedTheWait(t *testing.T) {
	never := context.Background()

	sigCh := make(chan os.Signal, 1)
	sigCh <- syscall.SIGTERM
	require.NoError(t, awaitShutdown(never, never, sigCh))

	errListener := errors.New("sample listener failure")
	listeners, failListener := context.WithCancelCause(context.Background())
	failListener(errListener)
	require.ErrorIs(t, awaitShutdown(listeners, never, sigCh), errListener)

	errComponent := errors.New("sample component failure")
	components, failComponent := context.WithCancelCause(context.Background())
	failComponent(errComponent)
	require.ErrorIs(t, awaitShutdown(never, components, sigCh), errComponent)
}

var (
	bootstrapOnce   sync.Once
	errBootstrap    error
	bootstrapLogDir string
	// runCovered records that Run has been driven once in this process.
	runCovered bool
)

// bootstrapProcess bootstraps the test process — on the scratch log
// directory TestMain made, a free port and the default in-memory database —
// and fails the test when that failed. Bootstrap is single-shot, so every
// test that needs a bootstrapped process shares the one bootstrap.
func bootstrapProcess(t *testing.T) {
	t.Helper()

	bootstrapOnce.Do(func() {
		port, err := freePort()
		if err != nil {
			errBootstrap = err
			return
		}
		t.Setenv(config.LOGGER_DIR, bootstrapLogDir)
		t.Setenv(config.DATABASE_AUTO_MIGRATE, "true")
		t.Setenv(config.SERVER_LISTEN, "127.0.0.1")
		t.Setenv(config.SERVER_PORT, strconv.Itoa(port))
		errBootstrap = Bootstrap()
	})
	require.NoError(t, errBootstrap)
}

// freePort asks the kernel for an unused port by binding to port zero and
// closing again.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()

	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.Newf("unexpected listener address %T", l.Addr())
	}
	return addr.Port, nil
}

// readyz asks the bootstrapped server's readiness probe and returns the
// status, or zero when the listener does not answer.
func readyz() int {
	cli := &http.Client{Timeout: 200 * time.Millisecond}
	addr := net.JoinHostPort(config.App.Server.Listen, strconv.Itoa(config.App.Server.Port))
	resp, err := cli.Get("http://" + addr + "/-/readyz")
	if err != nil {
		return 0
	}
	resp.Body.Close()
	return resp.StatusCode
}
