package bootstrap

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/lease"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/router"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
)

// lostLeaseHelper marks the child process that runs Run for a test of work
// that will not stop under a lost lease; the child runs that test alone.
const lostLeaseHelper = "GST_BOOTSTRAP_LOST_LEASE_HELPER"

// stubbornWork is the name of the lease the stubborn work runs under.
const stubbornWork = "sample-stubborn-work"

// TestRunEndsAtOnceWhenWorkUnderALostLeaseWillNotStop proves the one failure
// the shutdown does not wait on: work still running once its lease was lost
// and its grace has passed runs beside the replica that took the lease over,
// so Run returns that failure — the process exits with it — without waiting
// out the drain delay, a request still in flight or the components' own
// work. Run is single-shot and its cleanup unwinds the process, so the run
// happens in a child process: the test binary runs itself again with this
// test selected and the helper marked.
func TestRunEndsAtOnceWhenWorkUnderALostLeaseWillNotStop(t *testing.T) {
	if os.Getenv(lostLeaseHelper) == "1" {
		runLosingALeaseToWorkThatWillNotStop(t)
		return
	}
	runLostLeaseChild(t, "TestRunEndsAtOnceWhenWorkUnderALostLeaseWillNotStop")
}

// TestRunFailsWhenALeaseIsLostDuringTheShutdown proves the same failure
// coming while a shutdown is already under way ends the process as a failure
// too: a termination signal began a graceful shutdown, the lease was lost
// while the work under it wound down, and the work will not stop — Run
// returns that failure instead of the clean stop the signal began, so the
// process exits with it.
func TestRunFailsWhenALeaseIsLostDuringTheShutdown(t *testing.T) {
	if os.Getenv(lostLeaseHelper) == "1" {
		runLosingALeaseDuringTheShutdown(t)
		return
	}
	runLostLeaseChild(t, "TestRunFailsWhenALeaseIsLostDuringTheShutdown")
}

// runLostLeaseChild runs the test named test again in a child process with
// the helper marked, and fails unless the child ran it and passed.
func runLostLeaseChild(t *testing.T, test string) {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^"+test+"$", "-test.v")
	cmd.Env = append(os.Environ(), lostLeaseHelper+"=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "the child process must pass:\n%s", out)
	require.Contains(t, string(out), "--- PASS: "+test, "the child must have run the helper:\n%s", out)
}

// runLosingALeaseToWorkThatWillNotStop is the child's half: it brings the
// process up with a component holding a lease for work that never returns,
// a request in flight and a drain delay configured, has another holder take
// the lease over, and checks how soon Run ends once the process failed.
func runLosingALeaseToWorkThatWillNotStop(t *testing.T) {
	t.Helper()
	bootstrapProcess(t)
	// The protocol played out in milliseconds: the renewal after the
	// takeover finds the lease gone, and the grace passes at once.
	t.Cleanup(lease.SetTimings(600*time.Millisecond, 100*time.Millisecond, 300*time.Millisecond, 200*time.Millisecond))
	config.App.Server.ShutdownDelay = 5 * time.Second

	claimed, _ := registerStubbornWork(t)
	inFlight := make(chan struct{})
	router.Pub().GET("/sample-in-flight", func(*gin.Context) {
		close(inFlight)
		select {}
	})

	done := make(chan error, 1)
	go func() { done <- Run() }()
	require.Eventually(t, func() bool { return readyz() == http.StatusOK },
		10*time.Second, 20*time.Millisecond, "the server never came up")
	require.NoError(t, <-claimed)

	go func() {
		addr := net.JoinHostPort(config.App.Server.Listen, strconv.Itoa(config.App.Server.Port))
		if resp, err := http.Get("http://" + addr + "/api/sample-in-flight"); err == nil { //nolint:noctx // The request is meant to stay in flight.
			resp.Body.Close()
		}
	}()
	select {
	case <-inFlight:
	case <-time.After(5 * time.Second):
		t.Fatal("the request never reached its handler")
	}

	takeOverStubbornWork(t)
	select {
	case <-lifecycle.Failure().Done():
	case <-time.After(5 * time.Second):
		t.Fatal("work that will not stop under a lost lease must fail the process")
	}
	failed := time.Now()
	select {
	case err := <-done:
		require.ErrorContains(t, err, `lease "sample-stubborn-work" was lost and the work under it has not stopped`)
		require.Less(t, time.Since(failed), 3*time.Second, "Run must end without waiting on anything")
	case <-time.After(20 * time.Second):
		t.Fatal("Run waited on the shutdown although work under a lost lease will not stop")
	}
}

// runLosingALeaseDuringTheShutdown is the child's half: it brings the process
// up with a component holding a lease for work that never returns, sends the
// process a termination signal, has another holder take the lease over once
// the work is winding down, and checks what Run returns.
func runLosingALeaseDuringTheShutdown(t *testing.T) {
	t.Helper()
	bootstrapProcess(t)
	t.Cleanup(lease.SetTimings(600*time.Millisecond, 100*time.Millisecond, 300*time.Millisecond, 200*time.Millisecond))

	claimed, stopping := registerStubbornWork(t)
	done := make(chan error, 1)
	go func() { done <- Run() }()
	require.Eventually(t, func() bool { return readyz() == http.StatusOK },
		10*time.Second, 20*time.Millisecond, "the server never came up")
	require.NoError(t, <-claimed)

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))
	select {
	case <-stopping:
	case <-time.After(5 * time.Second):
		t.Fatal("the shutdown never reached the work")
	}
	takeOverStubbornWork(t)
	select {
	case err := <-done:
		require.ErrorContains(t, err, `lease "sample-stubborn-work" was lost and the work under it has not stopped`,
			"the shutdown the signal began ends with the failure that came during it")
	case <-time.After(20 * time.Second):
		t.Fatal("Run never returned")
	}
}

// registerStubbornWork registers a component that claims the stubbornWork
// lease as it starts and runs work under it that never returns. claimed
// reports the claim; stopping is closed once the work's context has ended.
func registerStubbornWork(t *testing.T) (claimed <-chan error, stopping <-chan struct{}) {
	t.Helper()

	log := pkgzap.Fallback(stubbornWork)
	claims, ended, returned := make(chan error, 1), make(chan struct{}), make(chan struct{})
	lifecycle.Register(lifecycle.Component{
		Name:  "sample-lease-holder",
		Stage: lifecycle.StageComponent,
		Start: func(ctx context.Context) error {
			h, won, err := lease.Claim(ctx, stubbornWork)
			if err == nil && !won {
				err = errors.New("the sample lease was held already")
			}
			if err != nil {
				claims <- err
				return err
			}
			// The renewals are never stopped: the work never returns.
			held, _ := lease.Hold(ctx, h, log)
			go func() {
				defer close(returned)
				_ = lease.Run(held, h, log, func(ctx context.Context) error {
					<-ctx.Done()
					close(ended)
					select {}
				})
			}()
			claims <- nil
			return nil
		},
		Stop: func(ctx context.Context) error {
			select {
			case <-returned:
				return nil
			case <-ctx.Done():
				return errors.Wrap(ctx.Err(), "gave up waiting for the sample work")
			}
		},
	})
	return claims, ended
}

// takeOverStubbornWork has another holder take the stubbornWork lease over
// behind the component's back.
func takeOverStubbornWork(t *testing.T) {
	t.Helper()

	require.NoError(t, dbruntime.DB.Exec("UPDATE gst_leases SET expires_at_ms = 0 WHERE name = ?", stubbornWork).Error)
	_, won, err := lease.Claim(context.Background(), stubbornWork)
	require.NoError(t, err)
	require.True(t, won)
}
