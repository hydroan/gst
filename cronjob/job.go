package cronjob

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/lease"
	"github.com/hydroan/gst/internal/lifecycle"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/util"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// This file holds one job's own side of the scheduler: the loop that waits
// for its instants, the claim that decides which replica runs one — or runs
// a round cut short a second time — and the round itself: its identity, its
// timing, the entry that records its outcome and the record of its end.
// Registration, the scheduler that owns the loops and the sweep that finds
// rounds cut short are in cronjob.go, the schedule arithmetic in schedule.go.

// releaseTimeout bounds the statement that gives an instant's lease back,
// and records whether its round ran to its end, once the round is over.
const releaseTimeout = 5 * time.Second

// job is one registered job with its parsed schedule.
type job struct {
	name     string
	spec     string
	fn       func(ctx context.Context) error
	schedule cron.Schedule
	// perInstance marks a job every replica runs on its own, without a
	// lease; the default job runs once per instant across the deployment.
	perInstance bool
}

// leaseName is the coordinated name the job's instants are claimed under.
func (j *job) leaseName() string {
	return "cron:" + j.name
}

// loop runs the job at each instant of its schedule, starting with next,
// until ctx ends; a job under a lease first catches up the most recent
// instant no replica claimed, and runs a second time each round cut short
// that the sweep hands in on cutShort. The instant after a run — the
// catch-up and the second run included — is computed from the moment the run
// ended, so instants that passed while a run was in flight are skipped, never
// piled on top of it — a slow round must not multiply its downstream calls —
// and every skip is logged with the number of instants it cost, so a job that
// keeps overrunning its period does not quietly run less often. A schedule
// with no instant left — a day that never comes — ends the loop.
func (j *job) loop(ctx context.Context, next time.Time, cutShort <-chan lease.Unfinished) {
	if !j.perInstance {
		if ran, ok := j.catchUp(ctx); ok {
			next = j.nextAfter(ran)
		}
	}
	for ctx.Err() == nil {
		if next.IsZero() {
			log.Warnz("cronjob has no further instant", zap.String("name", j.name), zap.String("spec", j.spec))
			return
		}
		due, stop := clk.Timer(next)
		select {
		case <-due:
			j.runInstant(ctx, next, false)
			next = j.nextAfter(next)
		case u := <-cutShort:
			stop()
			if j.rerun(ctx, u) {
				next = j.nextAfter(u.Slot)
			}
		case <-ctx.Done():
			stop()
		}
	}
}

// nextAfter returns the instant to wait for once the round for ran ended:
// the first instant after now — never before ran itself, so a wall clock
// set back cannot hand the same instant out again — logging the instants
// the round overran.
func (j *job) nextAfter(ran time.Time) time.Time {
	after := clk.Now()
	if after.Before(ran) {
		after = ran
	}
	following := j.schedule.Next(after)
	if skipped := j.instantsBetween(ran, following); skipped > 0 {
		log.Warnz("cronjob skipped instants", zap.String("name", j.name), zap.String("spec", j.spec), zap.Int("skipped", skipped), zap.Time("after", ran), zap.Time("next", following))
	}
	return following
}

// instantsBetween counts the instants of the schedule after from and before
// to: the ones a run that ended after them skipped.
func (j *job) instantsBetween(from, to time.Time) int {
	skipped := 0
	for t := j.schedule.Next(from); !t.IsZero() && t.Before(to); t = j.schedule.Next(t) {
		skipped++
	}
	return skipped
}

// catchUp runs, once and on one replica, the most recent instant of the job
// that no replica claimed — what a rolling deployment or an outage owes the
// job — and returns that instant and whether a round ran for it. An instant
// claimed whose round was cut short is not caught up here: the sweep finds
// it, and the loop runs it a second time. The conditions, all of which must
// hold: the job has run before, so its lease row exists (a job never run
// starts with its next instant: the instants before its first deployment
// were never its to run); the most recent instant that passed lies within
// catchUpLookback; and no replica claimed that instant — the claim itself
// decides this last one, so replicas racing for the same catch-up settle it
// the way they settle any instant.
func (j *job) catchUp(ctx context.Context) (time.Time, bool) {
	prev, ok := previousInstant(j.schedule, clk.Now())
	if !ok {
		return time.Time{}, false
	}
	last, found, err := lease.LastSlot(ctx, j.leaseName())
	if err != nil {
		// A process told to stop as it starts is not a database failure.
		if ctx.Err() == nil {
			log.Errorz("cronjob could not read its last instant", zap.Error(err), zap.String("name", j.name), zap.String("spec", j.spec))
		}
		return time.Time{}, false
	}
	if !found || !last.Before(prev) {
		return time.Time{}, false
	}
	return prev, j.runInstant(ctx, prev, true)
}

// runInstant runs the round for at and reports whether a round ran. A
// per-instance job runs it outright; a job shared across the deployment
// first claims the instant's lease and runs only when it wins, see
// runClaimed. A claim that gives up an earlier instant whose round was cut
// short — and not run a second time yet, or cut short the second time too —
// says so: that instant never runs again.
func (j *job) runInstant(ctx context.Context, at time.Time, catchUp bool) bool {
	if j.perInstance {
		// The round logs its own outcome.
		_ = j.run(ctx, at)
		return true
	}

	h, claimed, err := lease.ClaimSlot(ctx, j.leaseName(), at)
	if !j.claimed(ctx, at, claimed, err) {
		return false
	}
	if given, ok := h.Superseded(); ok {
		log.Warnz("cronjob gave up a round cut short", zap.String("name", j.name), zap.String("spec", j.spec), zap.Time("at", given.Slot), zap.Bool("rerun", given.Rerun), zap.Time("next", at))
	}
	var fields []zap.Field
	if catchUp {
		fields = append(fields, zap.Bool("catch_up", true))
	}
	j.runClaimed(ctx, h, at, fields...)
	return true
}

// rerun runs a second time the round for u, an instant of the job cut short,
// when it wins the instant's second claim, and reports whether a round ran.
func (j *job) rerun(ctx context.Context, u lease.Unfinished) bool {
	h, claimed, err := lease.ClaimRerun(ctx, u)
	if !j.claimed(ctx, u.Slot, claimed, err, zap.Bool("rerun", true)) {
		return false
	}
	j.runClaimed(ctx, h, u.Slot, zap.Bool("rerun", true))
	return true
}

// claimed reports whether the claim of the instant at was won, and logs the
// claim that was not: refused, or failed. fields are added to the entry.
func (j *job) claimed(ctx context.Context, at time.Time, claimed bool, err error, fields ...zap.Field) bool {
	entry := append([]zap.Field{zap.String("name", j.name), zap.String("spec", j.spec), zap.Time("at", at)}, fields...)
	switch {
	case err != nil:
		// An instant that falls on the moment the process is told to stop
		// is not claimed, and that is not a database failure.
		if ctx.Err() == nil {
			log.Errorz("cronjob could not claim its instant", append([]zap.Field{zap.Error(err)}, entry...)...)
		}
		return false
	case !claimed:
		log.Debugz("cronjob instant claimed elsewhere", entry...)
		return false
	default:
		return true
	}
}

// runClaimed runs the round for at under h, the lease claimed for it: its
// context ends with the lease, its database.Transaction calls verify the
// lease first, and a round that will not stop once the lease is lost fails
// the process, see lease.Run. The round then records its end, and gives the
// lease back so the next instant is free at once: a round that ran to its
// end — the job returned nil or an error of its own, or panicked — is
// recorded finished, and never runs again; one cut short — the job returned
// the ending of its context — stays unfinished, for a replica to run a second
// time. A round whose end the database fails to record stays unfinished too,
// holding its lease until it expires, and runs a second time like one cut
// short. fields are added to the round's entries.
func (j *job) runClaimed(ctx context.Context, h *lease.Handle, at time.Time, fields ...zap.Field) {
	fields = append([]zap.Field{zap.Uint64("term", h.Term())}, fields...)
	held, stopHold := lease.Hold(ctx, h, log)
	// The round logs its own outcome; Run's is the same error, already logged.
	runErr := lease.Run(lease.WithHandle(held, h), h, log, func(ctx context.Context) error {
		return j.run(ctx, at, fields...)
	})
	// Read before the renewals stop: stopping them ends the held context
	// too, and a round that returned a cancellation of its own — a context
	// it derived and ended itself — would then read as cut short.
	finished := !lifecycle.Interrupted(held, runErr)
	lost := h.Lost()
	stopHold()

	if lost && finished {
		// The round outlived its lease — the renewals could not keep it, or
		// found it taken — and returned nothing, or a failure of its own. A
		// job that returned the ending of its context has the loss on its own
		// entry already; this one has it recorded here.
		log.Warnz("cronjob lost its lease during the round", append([]zap.Field{zap.String("name", j.name), zap.String("spec", j.spec), zap.Time("at", at)}, fields...)...)
	}

	// The record outlives the round's context on purpose: at shutdown that
	// context is already gone, and the lease must still be handed back so
	// another replica can take the next instant, or the round cut short,
	// without waiting the lease out.
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()
	switch {
	case finished:
		// Recorded with the lease lost too: the round did run to its end.
		err := h.Finish(settleCtx)
		switch {
		case errors.Is(err, lease.ErrLost):
			log.Warnz("cronjob could not release its lease", zap.Error(err), zap.String("name", j.name), zap.Time("at", at))
		case err != nil:
			log.Warnz("cronjob could not record its round as finished", zap.Error(err), zap.String("name", j.name), zap.Time("at", at))
		}
	case lost:
		// Cut short, and the name is no longer this round's to give back:
		// the instant stays unfinished for another replica to run.
	default:
		if err := h.Release(settleCtx); err != nil {
			log.Warnz("cronjob could not release its lease", zap.Error(err), zap.String("name", j.name), zap.Time("at", at))
		}
	}
}

// run executes the round scheduled for at and returns its outcome, logged
// already. Round identity, panic recovery, timing and outcome logging live
// here; fields are added to every outcome entry.
//
// A failure goes out as a typed error field, never formatted into the
// message: the logging layer derives error_stack from that field, and for a
// job this entry is the only record of the failure, so it has to locate the
// failing line and not just name the job. Every outcome entry carries the
// round's trace id — the id the round's statements and log lines carry too —
// so the round is found again from any of them.
func (j *job) run(ctx context.Context, at time.Time, fields ...zap.Field) (runErr error) {
	ctx, traceID, end := beginRound(ctx, j.name)
	round := append([]zap.Field{zap.String("name", j.name), zap.String("spec", j.spec), zap.String(consts.TRACE_ID, traceID), zap.Time("at", at)}, fields...)
	// Registered before the recovery below so that it runs after it: a
	// panic is recorded on the round's span as its outcome.
	defer func() { end(runErr, interruption(ctx, runErr)) }()
	defer func() {
		if r := recover(); r != nil {
			runErr = util.PanicError(r)
			log.Errorz("cronjob panicked", append([]zap.Field{zap.Error(runErr)}, round...)...)
		}
	}()
	begin := time.Now()
	runErr = j.fn(ctx)
	round = append(round, util.LogDuration(time.Since(begin)))
	switch reason := interruption(ctx, runErr); {
	case runErr == nil:
		log.Infoz("finished cronjob", round...)
	case reason != "":
		log.Warnz("cronjob interrupted", append([]zap.Field{zap.String("reason", reason)}, round...)...)
	default:
		log.Errorz("finished cronjob with error", append([]zap.Field{zap.Error(runErr)}, round...)...)
	}
	return runErr
}

// interruption names why a round ended before its work did — the lease was
// lost, also while the round wound down once the process began shutting
// down, or the process is shutting down — and is empty for a round that
// ended on its own, a failure of its own included. A job stopping because
// its context ended is doing what it is asked to do then, not failing; a
// rolling deployment ends a long round this way every time. What counts is
// decided by lifecycle.Interrupted: the context's own ending — its error or
// its cause — wrapped or not, and nothing else — a job that also reports a
// failure of its own failed, and the entry carries that failure.
func interruption(ctx context.Context, err error) string {
	if !lifecycle.Interrupted(ctx, err) {
		return ""
	}
	// The handle, not the context's cause: a loss found after a shutdown
	// ended the context leaves the cause the shutdown's.
	if h, ok := lease.FromContext(ctx); ok && h.Lost() {
		return "lease lost"
	}
	return "shutting down"
}

// beginRound opens one round of the named job on parent and returns the
// context the job runs on, the round's trace id, and the function that
// closes the round with its outcome: the error the job returned, and the
// interruption that ended the round, if one did.
//
// The context carries the round's identity — the job name and the trace id —
// for everything the job does downstream: statement comments, the SQL log and
// the business log annotate themselves with it, the way they do with a
// request's. With tracing on the round also gets a root span, the parent of
// every span the job's operations open, and the trace id is that span's; with
// tracing off the id is generated, the way the request middleware generates
// one.
//
// The span's status says what the log entry says: a round that finished is
// ok, one that failed is an error, and one that was interrupted is neither —
// it carries an event naming the interruption and leaves the status unset,
// so a trace search for failed rounds does not turn up every deployment.
func beginRound(parent context.Context, name string) (ctx context.Context, traceID string, end func(err error, interruption string)) {
	ctx = parent
	var span trace.Span
	if gstotel.IsEnabled() {
		ctx, span = gstotel.StartSpan(ctx, gstotel.OperationSpanName("cronjob", name))
		traceID = span.SpanContext().TraceID().String()
	} else {
		traceID = util.TraceID()
	}
	ctx = execctx.WithCronjob(ctx, name, traceID)

	return ctx, traceID, func(err error, interruption string) {
		if span == nil {
			return
		}
		if gstotel.IsSpanRecording(span) {
			switch {
			case err == nil:
				span.SetStatus(codes.Ok, "")
			case interruption != "":
				span.AddEvent("interrupted", trace.WithAttributes(attribute.String("reason", interruption)))
			default:
				span.SetStatus(codes.Error, err.Error())
				gstotel.RecordError(span, err)
			}
		}
		span.End()
	}
}
