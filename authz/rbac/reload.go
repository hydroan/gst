package rbac

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/logger"
	prommetrics "github.com/hydroan/gst/metrics"
	"go.uber.org/zap"
)

// ReloadPolicies rebuilds the whole in-memory model from storage.
func (r *rbac) ReloadPolicies(ctx context.Context) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "reload_policies", nil)
	defer func() {
		finishSpan(err)
	}()

	return r.reload(ctx)
}

// reload rebuilds the whole in-memory policy set from storage.
//
// The read is held under the write lock, which stalls every authorization in
// the process for as long as it takes. That is deliberate, and it was arrived
// at the other way round: reading outside the lock and installing the result
// afterwards was tried and reverted. The applied sequence counts only the
// batches this process wrote, so it cannot order one reload against another,
// and two overlapping reloads each pass a check against it — leaving the slower
// one to install its older snapshot over the newer. Storage changed by anything
// other than this process, which is what ReloadPolicies exists for, is exactly
// where nothing local would have moved that sequence to catch it.
//
// Holding the lock also keeps the stale set from being served while its
// replacement is read. A reload usually follows a revocation that memory
// missed, so the decisions that window would serve are the ones already known
// to be wrong, and making requests wait is the safer way to be unavailable.
//
// The wait is bounded here, where the lock is taken, rather than left to each
// caller. Whatever asked for the reload — a caller with no deadline included —
// every authorization in the process is behind this lock, and a database that
// has stopped answering must cost them reloadTimeout at most, not forever. A
// caller whose own deadline is sooner keeps it; WithTimeout only ever tightens.
//
// The enforcer's own load is what makes this usable at all: it swaps the model
// through applyModifiedModel rather than SetModel. SetModel re-initializes the
// enforcer, rebuilding the function map so that no decision can resolve the
// matcher function this package registers, and turning autosave back on so that
// Casbin writes policies behind mutate's back.
func (r *rbac) reload(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(contextOrBackground(ctx), reloadTimeout)
	defer cancel()

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.enforcer.LoadPolicyCtx(ctx); err != nil {
		return errors.Wrap(err, "failed to reload casbin policies")
	}
	publishPolicyDivergence(false)
	return nil
}

// reloadTimeout bounds the enforcer write lock against a database that has
// stopped answering.
//
// It is pulled between two limits. It has to outlast the client whose request
// triggered a recovery reload, which is the whole reason that context is
// detached. It also has to stay well under what anything upstream will wait
// for, because a reload holds the enforcer write lock while it reads: every
// authorization in the process waits behind it. A read of the policy table is
// sub-second on a database that is answering, so this only ever bites on one
// that is not, and holding a request and the whole authorization path hostage
// for longer helps nobody.
const reloadTimeout = 10 * time.Second

// recoverPolicies rebuilds the model from storage after an in-memory update was
// overtaken or failed, and records whether the rebuild rescued this process.
//
// This is the one place where the decisions a process serves can disagree with
// what is stored: the transaction is already durable, so the write cannot be
// undone, and only memory is behind. Nothing downstream notices — the request
// that caused it has usually returned, and comparing stored rules against the
// records they come from cannot see a divergence that exists only in memory. It
// is recorded here because here is the only place that knows.
//
// The reload is given a context of its own. This runs from an after-commit
// action, which receives the context from before the transaction opened — in an
// HTTP handler, the request's. A client that has already disconnected would
// otherwise cancel the one read that can put this process back in step with
// storage, which is how a reload comes to fail most often. The reload bounds
// its own duration, so detaching the cancellation does not unbound it.
func (r *rbac) recoverPolicies(ctx context.Context, cause error) error {
	ctx = context.WithoutCancel(contextOrBackground(ctx))

	reloaded := r.reload(ctx)
	if reloaded == nil {
		if cause != nil {
			logger.Authz.Warnz(
				"rbac in-memory policy update failed, reloaded from storage",
				zap.Error(cause),
			)
		}
		return cause
	}

	publishPolicyDivergence(true)
	r.scheduleReloadRetry()
	// A nil cause is a batch that was deliberately not replayed — overtaken, or
	// a removal whose two halves disagreed — and each of those already logged
	// what happened. Saying "the update failed" for it would send whoever reads
	// this looking for a failure that never happened, and zap drops a nil error
	// field, so that reading would arrive with nothing at all to explain itself.
	if cause == nil {
		logger.Authz.Errorz(
			"rbac policy write could not be replayed in memory and the reload it needed failed, "+
				"authorization decisions now disagree with stored policies until this process reloads them",
			zap.NamedError("reload_error", reloaded),
		)
		return reloaded
	}
	logger.Authz.Errorz(
		"rbac in-memory policy update failed and could not be reloaded, "+
			"authorization decisions now disagree with stored policies until this process reloads them",
		zap.Error(cause),
		zap.NamedError("reload_error", reloaded),
	)
	return errors.Join(cause, reloaded)
}

// policiesDiverged reports whether this process serves authorization decisions
// from an in-memory policy set that no longer agrees with storage.
var policiesDiverged atomic.Bool

// reloadRetryInterval is how long a diverged process waits between its attempts
// to put itself back in step with storage. Each attempt is one bounded read of
// the policy table, so the interval trades nothing but how long known-wrong
// decisions keep being served against load on a database that is likely
// already struggling. It is a variable only so tests do not wait it out.
var reloadRetryInterval = 10 * time.Second

// reloadRetryRunning keeps the retry to one goroutine however many failed
// recoveries pile up while the database is away.
var reloadRetryRunning atomic.Bool

// scheduleReloadRetry keeps retrying the reload a failed recovery could not
// perform, until the process agrees with storage again.
//
// It exists because a failed recovery otherwise ends the story: the divergence
// is published and logged, and then nothing ever tries again — a process that
// missed one revocation while the database blipped would keep allowing it for
// the rest of its life. The retry is driven by the divergence state rather
// than by a schedule of its own, so a process in step with storage runs no
// goroutine and reads nothing.
//
// A reload that succeeds anywhere — here, or a recovery on another write —
// clears the state this loop is watching, and the loop ends with it.
func (r *rbac) scheduleReloadRetry() {
	if !reloadRetryRunning.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer reloadRetryRunning.Store(false)
		for {
			time.Sleep(reloadRetryInterval)
			if !policiesDiverged.Load() {
				return
			}
			err := r.reload(context.Background())
			if err == nil {
				logger.Authz.Infoz(
					"rbac policy set reloaded after divergence, decisions agree with storage again",
				)
				return
			}
			logger.Authz.Warnz(
				"rbac policy reload retry failed, authorization decisions still disagree with stored policies",
				zap.Error(err),
			)
		}
	}()
}

// publishPolicyDivergence records whether this process is still in step with
// storage, so that something outside it can act on the answer.
func publishPolicyDivergence(diverged bool) {
	policiesDiverged.Store(diverged)

	// The gauge is created by prommetrics.Init, which bootstrap runs long
	// before anything can write a policy. A process that never ran bootstrap,
	// which is what a test is, still has to be able to reload.
	if prommetrics.AuthzPolicyDiverged == nil {
		return
	}
	if diverged {
		prommetrics.AuthzPolicyDiverged.Set(1)
		return
	}
	prommetrics.AuthzPolicyDiverged.Set(0)
}
