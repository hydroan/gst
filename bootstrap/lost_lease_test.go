package bootstrap

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
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

// lostLeaseHelper marks the child process that runs Run for
// TestRunEndsAtOnceWhenWorkUnderALostLeaseWillNotStop.
const lostLeaseHelper = "GST_BOOTSTRAP_LOST_LEASE_HELPER"

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

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunEndsAtOnceWhenWorkUnderALostLeaseWillNotStop$", "-test.v")
	cmd.Env = append(os.Environ(), lostLeaseHelper+"=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "the child process must pass:\n%s", out)
	require.Contains(t, string(out), "--- PASS: TestRunEndsAtOnceWhenWorkUnderALostLeaseWillNotStop", "the child must have run the helper:\n%s", out)
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

	const name = "sample-stubborn-work"
	log := pkgzap.Fallback(name)
	claimed, returned := make(chan error, 1), make(chan struct{})
	lifecycle.Register(lifecycle.Component{
		Name:  "sample-lease-holder",
		Stage: lifecycle.StageComponent,
		Start: func(ctx context.Context) error {
			h, won, err := lease.Claim(ctx, name)
			if err == nil && !won {
				err = errors.New("the sample lease was held already")
			}
			if err != nil {
				claimed <- err
				return err
			}
			// The renewals are never stopped: the work never returns.
			held, _ := lease.Hold(ctx, h, log)
			go func() {
				defer close(returned)
				_ = lease.Run(held, h, log, func(context.Context) error {
					select {}
				})
			}()
			claimed <- nil
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

	// Another holder takes the lease over behind the component's back.
	require.NoError(t, dbruntime.DB.Exec("UPDATE gst_leases SET expires_at_ms = 0 WHERE name = ?", name).Error)
	_, won, err := lease.Claim(context.Background(), name)
	require.NoError(t, err)
	require.True(t, won)

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
