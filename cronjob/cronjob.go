// Package cronjob runs the jobs a project registers on a schedule.
//
// A job is a func(ctx) error registered from a package init function with a
// cron expression — six fields, seconds first — or a descriptor such as
// "@hourly" or "@every 5m". Importing the package is what enables scheduling:
// the scheduler joins the process lifecycle from init, starts once the tables
// are ready and stops as soon as the process begins to drain; a project that
// registers no job never links it.
//
// Every schedule is read in UTC, and "@every" runs on the multiples of its
// period counted from the Unix epoch, so every replica of a deployment
// computes the same instants for a job — the ground a cluster-wide "once per
// instant" rule stands on. An expression that must follow another wall clock
// says so itself, with a CRON_TZ= prefix.
//
// Each job runs in a loop of its own: the loop waits for the next instant of
// the schedule, runs the job on a context that ends when the process begins
// shutting down, then computes the next instant from the moment the run
// ended — an instant that passed while a run was still in flight is skipped,
// never piled on top of it. The parser is the cron library's; the loop is
// this package's, because the library's runtime neither tells a job which
// instant it runs for nor lets a test drive the clock.
//
// A registration that cannot be honored — no name, no schedule, a schedule
// that does not parse, a name already taken — fails the process at startup
// rather than dropping the job: the name is the job's identity in logs and,
// under a lease, its key.
package cronjob

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	pkgzap "github.com/hydroan/gst/logger/zap"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/util"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// parser reads the schedules: six fields with seconds first, plus the
// descriptors — the syntax the scaffold documents.
var parser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

var (
	mu sync.Mutex
	// jobs holds every registration, in registration order.
	jobs []*job
	// errRegister collects the registrations that could not be honored;
	// start reports them, so the process fails at startup instead of
	// silently running without those jobs.
	errRegister error
	log         types.Logger
	// clk is the time the scheduler goes by. Tests swap in one they drive
	// by hand, so a schedule of hours plays out in microseconds.
	clk clock = systemClock{}

	// current is the started scheduler, nil until start. A registration
	// after that would never be scheduled, so it fails fast instead.
	current *scheduler
)

// job is one registered job with its parsed schedule.
type job struct {
	name     string
	spec     string
	fn       func(ctx context.Context) error
	schedule cron.Schedule
}

// scheduler is one started scheduler: the loops it runs and what ends them.
type scheduler struct {
	// cancel ends the context every loop runs on; stop calls it, and the
	// process context ending does the same.
	cancel context.CancelFunc
	loops  sync.WaitGroup
	// done closes once every loop has returned.
	done chan struct{}
}

func init() {
	// Importing this package is what enables scheduling: through the
	// lifecycle registry, bootstrap starts the scheduler once the tables are
	// ready and stops it as soon as the process begins to drain. A project
	// that registers no job never imports the package and never runs a
	// scheduler.
	lifecycle.Register(lifecycle.Component{Name: "cronjob", Stage: lifecycle.StageComponent, Start: start, Stop: stop})
}

// Register declares fn as the job named name, run on spec: a six-field cron
// expression, seconds first, or a descriptor such as "@hourly" or "@every
// 5m". Schedules are read in UTC — an expression that must follow another
// wall clock carries a CRON_TZ= prefix — and "@every" runs on the multiples
// of its period from the Unix epoch, so every replica computes the same
// instants. Registration belongs in package init functions: the scheduler
// starts with the process, and a registration after that panics.
//
// fn receives the context of the round it runs in. The context ends when the
// process begins shutting down, so a long round can stop early; it carries
// the round's identity — the job name and a trace id of the round's own, see
// execctx — and, with tracing on, the round's root span, so every statement,
// log line and span the job produces is annotated with the round and can be
// found again from any of them. An instant that passes while the previous
// round is still in flight is skipped.
//
// A registration that cannot be honored — no name, no schedule, a schedule
// that does not parse, a name already taken, a nil fn — fails the process at
// startup.
func Register(fn func(ctx context.Context) error, spec string, name string) {
	mu.Lock()
	defer mu.Unlock()

	if current != nil {
		panic(fmt.Sprintf("cronjob: %q registered after the scheduler started; register jobs in package init functions", name))
	}
	j, err := newJob(fn, spec, name)
	if err != nil {
		errRegister = errors.Join(errRegister, err)
		return
	}
	jobs = append(jobs, j)
}

// newJob validates one registration and parses its schedule. The caller
// holds mu: the duplicate check reads jobs.
func newJob(fn func(ctx context.Context) error, spec, name string) (*job, error) {
	name = strings.TrimSpace(name)
	spec = strings.TrimSpace(spec)
	switch {
	case name == "":
		return nil, errors.Newf("cronjob: the job on schedule %q has no name", spec)
	case fn == nil:
		return nil, errors.Newf("cronjob %q: nil function", name)
	case spec == "":
		return nil, errors.Newf("cronjob %q: empty schedule", name)
	case slices.ContainsFunc(jobs, func(j *job) bool { return j.name == name }):
		return nil, errors.Newf("cronjob %q: registered twice", name)
	}

	parsed, err := parser.Parse(spec)
	if err != nil {
		return nil, errors.Wrapf(err, "cronjob %q: invalid schedule %q", name, spec)
	}
	return &job{name: name, spec: spec, fn: fn, schedule: inUTC(parsed)}, nil
}

// inUTC pins a parsed schedule to UTC: an expression that names no zone is
// read in UTC instead of the process's zone, and "@every" runs on the epoch
// grid instead of counting from the process start. Every replica then
// computes the same instants for a job.
func inUTC(s cron.Schedule) cron.Schedule {
	switch s := s.(type) {
	case *cron.SpecSchedule:
		// The parser leaves Location at time.Local for an expression that
		// names no zone; one with a CRON_TZ= prefix gets that zone, which
		// stays.
		if s.Location == time.Local {
			s.Location = time.UTC
		}
		return s
	case cron.ConstantDelaySchedule:
		return everySchedule{period: s.Delay}
	default:
		return s
	}
}

// everySchedule fires every period, on the multiples of the period counted
// from the Unix epoch: "@every 5m" runs at :00, :05, :10 on every replica
// alike, where counting from the process start — what the parser's own
// schedule does — puts each replica on a grid of its own.
type everySchedule struct {
	period time.Duration
}

// Next returns the first grid instant after t.
func (s everySchedule) Next(t time.Time) time.Time {
	period := s.period.Nanoseconds()
	if period <= 0 {
		return time.Time{}
	}
	return time.Unix(0, (t.UnixNano()/period+1)*period).UTC()
}

// start brings the scheduler up: a loop per registered job, each on a
// context derived from ctx. The context is the process context: its
// cancellation is the first sign of shutdown, and halts scheduling right
// then so no round starts while the process is on its way out; stop waits
// for the rounds already in flight. A registration that could not be
// honored fails the start.
func start(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()

	if errRegister != nil {
		return errRegister
	}
	if log == nil {
		// Adopt the shared cronjob logger so this package never opens a
		// second lumberjack instance on the same file, which would race its
		// rotation. Bootstrap initializes logging long before the scheduler
		// starts, so the local fallback only serves processes that never ran
		// the logging setup (e.g. unit tests).
		if log = logger.Cronjob; log == nil {
			log = pkgzap.New("cronjob.log")
		}
	}

	loopCtx, cancel := context.WithCancel(ctx)
	s := &scheduler{cancel: cancel, done: make(chan struct{})}
	now := clk.Now()
	for _, j := range jobs {
		next := j.schedule.Next(now)
		log.Infoz("scheduled cronjob", zap.String("name", j.name), zap.String("spec", j.spec), zap.Time("next", next))
		s.loops.Go(func() { j.loop(loopCtx, next) })
	}
	go func() {
		s.loops.Wait()
		close(s.done)
	}()
	current = s
	return nil
}

// stop halts scheduling, ends the context of every round in flight and
// waits for the rounds to return, for as long as ctx allows: a job that
// ignores its context cannot hold the shutdown hostage, and giving up on one
// is reported as the error bootstrap logs. Bootstrap runs it before the HTTP
// drain and before the connections jobs may still be using are closed;
// without it, shutdown would kill jobs mid-write. In a process that never
// started the scheduler it is a no-op.
func stop(ctx context.Context) error {
	mu.Lock()
	s := current
	mu.Unlock()
	if s == nil {
		return nil
	}
	s.cancel()

	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "gave up waiting for in-flight jobs")
	}
}

// loop runs the job at each instant of its schedule, starting with next,
// until ctx ends. The instant after a run is computed from the moment the
// run ended, so instants that passed while a run was in flight are skipped,
// never piled on top of it: a slow round must not multiply its downstream
// calls. A schedule with no instant left — a day that never comes — ends
// the loop.
func (j *job) loop(ctx context.Context, next time.Time) {
	for {
		if next.IsZero() {
			log.Warnz("cronjob has no further instant", zap.String("name", j.name), zap.String("spec", j.spec))
			return
		}
		if !clk.Wait(ctx, next) {
			return
		}
		j.run(ctx, next)

		// Never before the instant just run: a wall clock set back would
		// otherwise hand the same instant out again.
		after := clk.Now()
		if after.Before(next) {
			after = next
		}
		next = j.schedule.Next(after)
	}
}

// run executes the round scheduled for at. Round identity, panic recovery,
// timing and outcome logging live here.
//
// A failure goes out as a typed error field, never formatted into the
// message: the logging layer derives error_stack from that field, and for a
// job this entry is the only record of the failure, so it has to locate the
// failing line and not just name the job. Every outcome entry carries the
// round's trace id — the id the round's statements and log lines carry too —
// so the round is found again from any of them.
func (j *job) run(ctx context.Context, at time.Time) {
	ctx, traceID, end := beginRound(ctx, j.name)
	var runErr error
	// Registered before the recovery below so that it runs after it: a
	// panic is recorded on the round's span as its outcome.
	defer func() { end(runErr) }()
	defer func() {
		if r := recover(); r != nil {
			runErr = panicError(r)
			log.Errorz("cronjob panicked", zap.Error(runErr), zap.String("name", j.name), zap.String("spec", j.spec), zap.String(consts.TRACE_ID, traceID), zap.Time("at", at))
		}
	}()
	begin := time.Now()
	if runErr = j.fn(ctx); runErr != nil {
		log.Errorz("finished cronjob with error", zap.Error(runErr), zap.String("name", j.name), zap.String("spec", j.spec), zap.String(consts.TRACE_ID, traceID), zap.Time("at", at), util.LogDuration(time.Since(begin)))
	} else {
		log.Infoz("finished cronjob", zap.String("name", j.name), zap.String("spec", j.spec), zap.String(consts.TRACE_ID, traceID), zap.Time("at", at), util.LogDuration(time.Since(begin)))
	}
}

// beginRound opens one round of the named job on parent and returns the
// context the job runs on, the round's trace id, and the function that
// closes the round with its outcome.
//
// The context carries the round's identity — the job name and the trace id —
// for everything the job does downstream: statement comments, the SQL log and
// the business log annotate themselves with it, the way they do with a
// request's. With tracing on the round also gets a root span, the parent of
// every span the job's operations open, and the trace id is that span's; with
// tracing off the id is generated, the way the request middleware generates
// one.
func beginRound(parent context.Context, name string) (ctx context.Context, traceID string, end func(err error)) {
	ctx = parent
	var span trace.Span
	if gstotel.IsEnabled() {
		ctx, span = gstotel.StartSpan(ctx, gstotel.OperationSpanName("cronjob", name))
		traceID = span.SpanContext().TraceID().String()
	} else {
		traceID = util.TraceID()
	}
	ctx = execctx.WithCronjob(ctx, name, traceID)

	return ctx, traceID, func(err error) {
		if span == nil {
			return
		}
		if gstotel.IsSpanRecording(span) {
			if err != nil {
				span.SetStatus(codes.Error, err.Error())
				gstotel.RecordError(span, err)
			} else {
				span.SetStatus(codes.Ok, "")
			}
		}
		span.End()
	}
}

// panicError turns a recovered panic value into an error carrying the stack
// of the panic site. It must be called from the deferred function that
// recovered, while the goroutine is still unwinding: the frames captured then
// still include the line that panicked, whereas a stack taken after recovery
// would only show this package. A panic value that is already an error keeps
// its own, deeper stack if it has one — the error_stack field reports the
// deepest stack in the chain — and gains this one otherwise.
func panicError(recovered any) error {
	if err, ok := recovered.(error); ok {
		return errors.WithStack(err)
	}
	return errors.Newf("%v", recovered)
}

// clock is the time as the scheduler sees it: the moment now, and a wait
// until a moment. The scheduler never sleeps on its own, so a test can hand
// it a clock it drives by hand.
type clock interface {
	Now() time.Time
	// Wait blocks until t has passed or ctx ends, and reports whether t
	// passed.
	Wait(ctx context.Context, t time.Time) bool
}

// systemClock is the clock of a running process.
type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now()
}

func (systemClock) Wait(ctx context.Context, t time.Time) bool {
	if ctx.Err() != nil {
		return false
	}
	delay := time.Until(t)
	if delay <= 0 {
		return true
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
