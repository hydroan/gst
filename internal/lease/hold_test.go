package lease

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// TestHoldEndsWhenARenewalReportsTheLeaseLost proves the context Hold hands
// out ends, with ErrLost as its cause, the moment a renewal reports the
// lease gone.
func TestHoldEndsWhenARenewalReportsTheLeaseLost(t *testing.T) {
	ctx, stop := hold(context.Background(), "sample", func(context.Context) error { return ErrLost }, 5*time.Millisecond, time.Second)
	defer stop()

	awaitDone(ctx, t)
	require.ErrorIs(t, context.Cause(ctx), ErrLost)
}

// TestHoldEndsAtTheLocalDeadlineWhenRenewalsFail proves a holder that
// cannot reach the database gives itself up once the local deadline passes
// without a successful renewal — and not before: a single failure is not a
// loss.
func TestHoldEndsAtTheLocalDeadlineWhenRenewalsFail(t *testing.T) {
	const deadline = 60 * time.Millisecond
	begin := time.Now()
	ctx, stop := hold(context.Background(), "sample", func(context.Context) error { return errors.New("sample outage") }, 5*time.Millisecond, deadline)
	defer stop()

	awaitDone(ctx, t)
	require.ErrorIs(t, context.Cause(ctx), ErrLost)
	require.GreaterOrEqual(t, time.Since(begin), deadline, "one failed renewal must not end the lease before the deadline")
}

// TestHoldKeepsGoingWhileRenewalsSucceed proves successful renewals keep
// moving the deadline: the context outlives many deadlines' worth of time,
// and ends only when the holder stops it, without ErrLost.
func TestHoldKeepsGoingWhileRenewalsSucceed(t *testing.T) {
	ctx, stop := hold(context.Background(), "sample", func(context.Context) error { return nil }, 5*time.Millisecond, 30*time.Millisecond)

	select {
	case <-ctx.Done():
		t.Fatalf("the lease ended although every renewal succeeded: %v", context.Cause(ctx))
	case <-time.After(150 * time.Millisecond):
	}

	stop()
	awaitDone(ctx, t)
	require.NotErrorIs(t, context.Cause(ctx), ErrLost, "stopping the renewals is not a loss")
}

// TestHoldBoundsAHangingRenewal proves a renewal that never returns cannot
// carry the holder past its deadline: the attempt is cut at the deadline
// and the lease counts as lost.
func TestHoldBoundsAHangingRenewal(t *testing.T) {
	const deadline = 60 * time.Millisecond
	ctx, stop := hold(context.Background(), "sample", func(attempt context.Context) error {
		<-attempt.Done()
		return attempt.Err()
	}, 5*time.Millisecond, deadline)
	defer stop()

	awaitDone(ctx, t)
	require.ErrorIs(t, context.Cause(ctx), ErrLost)
}

// TestHoldEndsWithItsParent proves the work's context ends with the process
// context it derives from, so shutdown reaches the work under a lease.
func TestHoldEndsWithItsParent(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, stop := hold(parent, "sample", func(context.Context) error { return nil }, 5*time.Millisecond, time.Second)
	defer stop()

	cancelParent()
	awaitDone(ctx, t)
	require.ErrorIs(t, context.Cause(ctx), context.Canceled)
}

// awaitDone waits for ctx to end, failing the test when it does not in time.
func awaitDone(ctx context.Context, t *testing.T) {
	t.Helper()

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the context did not end")
	}
}
