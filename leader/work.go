package leader

import (
	"context"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/lease"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// This file holds one registration's own side of the elector: the loop that
// campaigns for its name, the tenure it leads once it wins, and the run of
// the work itself. Registration and the elector that owns the loops are in
// leader.go.

// The elector's timings. Variables so a test can play them out in
// milliseconds.
var (
	// campaignInterval is how long a replica waits between two claims of a
	// name held elsewhere, and before campaigning again once its own tenure
	// ended.
	campaignInterval = 5 * time.Second
	// campaignJitter bounds the random addition to campaignInterval that
	// keeps the replicas of a deployment from claiming in lockstep.
	campaignJitter = time.Second
)

// releaseTimeout bounds the statement that gives the name back once the work
// has returned.
const releaseTimeout = 5 * time.Second

// work is one registration: the name campaigned for and the function run
// by whoever wins it.
type work struct {
	name string
	fn   func(ctx context.Context) error
}

// leaseName is the coordinated name the work's leadership is claimed under.
func (w *work) leaseName() string {
	return "leader:" + w.name
}

// loop campaigns for the name until ctx ends: it claims the name, leads for
// as long as the claim holds when it wins, and waits the campaign interval
// between one attempt and the next, a tenure's end included — so work that
// returns at once does not spin, and a name held elsewhere is asked for
// again and again.
func (w *work) loop(ctx context.Context) {
	for {
		h, won, err := lease.Claim(ctx, w.leaseName())
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			log.Warnz("leader campaign failed", zap.Error(err), zap.String("name", w.name))
		case won:
			w.lead(ctx, h)
		}
		if !wait(ctx, campaignWait()) {
			return
		}
	}
}

// campaignWait is the wait between two campaign attempts: the interval plus
// a random share of the jitter, so the replicas of a deployment do not claim
// in lockstep.
func campaignWait() time.Duration {
	if campaignJitter <= 0 {
		return campaignInterval
	}
	return campaignInterval + rand.N(campaignJitter) //nolint:gosec // The jitter spreads the claims out; it is not a secret.
}

// wait blocks until d has passed or ctx ends, and reports whether d passed.
func wait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// lead runs the work under h until it returns or the tenure ends — the lease
// is lost, or ctx, the process, is shutting down — then gives the name back,
// unless the tenure ended because the name was no longer this holder's. The
// work runs on the tenure's context: it ends with the tenure, its
// database.Transaction calls verify the lease first, it carries the tenure's
// identity, and work that will not stop once the lease is lost fails the
// process, see lease.Run.
func (w *work) lead(ctx context.Context, h *lease.Handle) {
	held, stopHold := lease.Hold(ctx, h, log)
	traceID := util.TraceID()
	tenure := execctx.WithLeader(lease.WithHandle(held, h), w.name, traceID)
	fields := []zap.Field{zap.String("name", w.name), zap.Uint64("term", h.Term()), zap.String(consts.TRACE_ID, traceID)}
	log.Infoz("elected leader", fields...)

	begin := time.Now()
	err := lease.Run(tenure, h, log, w.run)
	// Read before the renewals stop: stopping them ends the held context
	// too, and would make every tenure look like a shutdown.
	reason, lost := tenureEnd(held)
	if lifecycle.Interrupted(held, err) {
		// The work returning the tenure's own cancellation is how a tenure
		// ends, not a failure of the work; a failure of its own beside the
		// cancellation is reported as one.
		err = nil
	}
	stopHold()

	outcome := append(slices.Clone(fields), zap.String("reason", reason), util.LogDuration(time.Since(begin)))
	if err != nil {
		log.Errorz("leader stepped down with error", append([]zap.Field{zap.Error(err)}, outcome...)...)
	} else {
		log.Infoz("leader stepped down", outcome...)
	}
	if lost {
		// The name is no longer this tenure's to give back.
		return
	}

	// The release outlives the tenure's context on purpose: at shutdown that
	// context is already gone, and the name must still be handed back so
	// another replica can take it without waiting the lease out.
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()
	if err := h.Release(releaseCtx); err != nil {
		log.Warnz("leader could not release its lease", append([]zap.Field{zap.Error(err)}, fields...)...)
	}
}

// tenureEnd names why a tenure ended, for its log entry, and whether it was
// the lease being lost: the alternatives are the process shutting down and
// the work returning on its own.
func tenureEnd(held context.Context) (reason string, lost bool) {
	switch {
	case errors.Is(context.Cause(held), lease.ErrLost):
		return "lease lost", true
	case held.Err() != nil:
		return "shutting down", false
	default:
		return "work returned", false
	}
}

// run executes the work on the tenure's context, recovering a panic into an
// error carrying the stack of the panic site: the leader log is the only
// record of the work's failure, so the entry has to locate the failing line
// and not just name the work.
func (w *work) run(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = util.PanicError(r)
			log.Errorz("leader work panicked", zap.Error(err), zap.String("name", w.name))
		}
	}()
	return w.fn(ctx)
}
