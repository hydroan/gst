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
// for its instants, the claim that decides which replica runs one, and the
// round itself — its identity, its timing and the entry that records its
// outcome. Registration and the scheduler that owns the loops are in
// cronjob.go, the schedule arithmetic in schedule.go.

// releaseTimeout bounds the statement that gives an instant's lease back
// once the round is over.
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
// instant no replica ran. The instant after a run — the catch-up included —
// is computed from the moment the run ended, so instants that passed while
// a run was in flight are skipped, never piled on top of it — a slow round
// must not multiply its downstream calls — and every skip is logged with the
// number of instants it cost, so a job that keeps overrunning its period
// does not quietly run less often. A schedule with no instant left — a day
// that never comes — ends the loop.
func (j *job) loop(ctx context.Context, next time.Time) {
	if !j.perInstance {
		if ran, ok := j.catchUp(ctx); ok {
			next = j.nextAfter(ran)
		}
	}
	for {
		if next.IsZero() {
			log.Warnz("cronjob has no further instant", zap.String("name", j.name), zap.String("spec", j.spec))
			return
		}
		if !clk.Wait(ctx, next) {
			return
		}
		j.runInstant(ctx, next, false)
		next = j.nextAfter(next)
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
// that no replica ran — what a rolling deployment or an outage owes the job
// — and returns that instant and whether a round ran for it. The conditions,
// all of which must hold: the job has run before, so its lease row exists (a
// job never run starts with its next instant: the instants before its first
// deployment were never its to run); the most recent instant that passed
// lies within catchUpLookback; and no replica claimed that instant — the
// claim itself decides this last one, so replicas racing for the same
// catch-up settle it the way they settle any instant.
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
// first claims the instant's lease and runs only when it wins, under the
// lease — its context ends with the lease, its database.Transaction calls
// verify the lease first, and a round that will not stop once the lease is
// lost fails the process, see lease.Run — then gives the lease back so the
// next instant is free at once.
func (j *job) runInstant(ctx context.Context, at time.Time, catchUp bool) bool {
	if j.perInstance {
		// The round logs its own outcome.
		_ = j.run(ctx, at)
		return true
	}

	h, claimed, err := lease.ClaimSlot(ctx, j.leaseName(), at)
	if err != nil {
		// An instant that falls on the moment the process is told to stop
		// is not claimed, and that is not a database failure.
		if ctx.Err() == nil {
			log.Errorz("cronjob could not claim its instant", zap.Error(err), zap.String("name", j.name), zap.String("spec", j.spec), zap.Time("at", at))
		}
		return false
	}
	if !claimed {
		log.Debugz("cronjob instant claimed elsewhere", zap.String("name", j.name), zap.String("spec", j.spec), zap.Time("at", at))
		return false
	}

	fields := []zap.Field{zap.Uint64("term", h.Term())}
	if catchUp {
		fields = append(fields, zap.Bool("catch_up", true))
	}
	held, stopHold := lease.Hold(ctx, h, log)
	// The round logs its own outcome; Run's is the same error, already logged.
	runErr := lease.Run(lease.WithHandle(held, h), h, log, func(ctx context.Context) error {
		return j.run(ctx, at, fields...)
	})
	lost := errors.Is(context.Cause(held), lease.ErrLost)
	stopHold()

	if lost {
		// The round outlived its lease — the renewals could not keep it, or
		// found it taken — and its context ended with it. A job that
		// returned the ending has the loss on its own entry already; one
		// that returned nothing, or a failure of its own, has it recorded
		// here. Either way the name is no longer this round's to give back.
		if !lifecycle.Interrupted(held, runErr) {
			log.Warnz("cronjob lost its lease during the round", zap.String("name", j.name), zap.String("spec", j.spec), zap.Time("at", at), zap.Uint64("term", h.Term()))
		}
		return true
	}

	// The release outlives the round's context on purpose: at shutdown that
	// context is already gone, and the lease must still be handed back so
	// another replica can take the next instant without waiting it out.
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()
	if err := h.Release(releaseCtx); err != nil {
		log.Warnz("cronjob could not release its lease", zap.Error(err), zap.String("name", j.name), zap.Time("at", at))
	}
	return true
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
// lost, or the process is shutting down — and is empty for a round that
// ended on its own, a failure of its own included. A job stopping because
// its context ended is doing what it is asked to do then, not failing; a
// rolling deployment ends a long round this way every time. What counts is
// decided by lifecycle.Interrupted: the context's own ending, wrapped or
// not, and nothing else — a job that also reports a failure of its own
// failed, and the entry carries that failure.
func interruption(ctx context.Context, err error) string {
	if !lifecycle.Interrupted(ctx, err) {
		return ""
	}
	if errors.Is(context.Cause(ctx), lease.ErrLost) {
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
