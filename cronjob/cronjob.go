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
// computes the same instants for a job. An expression that must follow
// another wall clock says so itself, with a CRON_TZ= prefix.
//
// A job runs once per instant across the deployment: the replicas share the
// instant's lease through the primary database (see the lease package), the
// first to claim it runs the round while the others skip the instant, and a
// round still holding the lease keeps the next instants from everyone. A job
// that must run on every replica — refreshing a process-local cache,
// cleaning a local directory — registers with RegisterPerInstance and runs
// without a lease; the lease table comes with the package all the same, and
// stays empty for such a project. A ClickHouse primary database cannot carry
// leases, so a job under a lease fails the start there.
//
// On start-up the scheduler catches up the most recent instant of a job when
// no replica ran it, which is what a rolling deployment or an outage owes
// the job. The rule, exactly: the job has run before (its lease row exists —
// a job never run starts with its next instant, the instants before its
// first deployment were never its to run); the most recent instant that
// passed lies within the last day (an older one is history, not a missed
// round); and no replica claimed that instant. The catch-up is one round, on
// one replica, and never repeats an instant already run.
//
// Each job runs in a loop of its own: the loop waits for the next instant of
// the schedule, runs the job on a context that ends when the process begins
// shutting down or the lease is lost, then computes the next instant from
// the moment the run ended — an instant that passed while a run was still
// in flight is skipped, never piled on top of it. The parser is the cron
// library's; the loop is this package's, because the library's runtime
// neither tells a job which instant it runs for nor lets a test drive the
// clock.
//
// A registration that cannot be honored — no name, no schedule, a schedule
// that does not parse or names the process's own zone, a name already taken —
// fails the process at startup rather than dropping the job: the name is the
// job's identity in every log line about it and the name of its lease.
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
	"github.com/hydroan/gst/internal/lease"
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

const (
	// catchUpLookback bounds how far back the start-up catch-up looks for a
	// job's most recent instant: one older than this is history, not a
	// missed round.
	catchUpLookback = 24 * time.Hour
	// releaseTimeout bounds the statement that gives an instant's lease back
	// once the round is over.
	releaseTimeout = 5 * time.Second
)

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
	// perInstance marks a job every replica runs on its own, without a
	// lease; the default job runs once per instant across the deployment.
	perInstance bool
}

// leaseName is the coordinated name the job's instants are claimed under.
func (j *job) leaseName() string {
	return "cron:" + j.name
}

// scheduler is one started scheduler: the jobs it runs, the loops it runs
// them in, and what ends them.
type scheduler struct {
	jobs []*job
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
// wall clock carries a CRON_TZ= prefix naming a fixed zone; Local, the
// process's own zone, is refused because replicas need not share it — and
// "@every" runs on the multiples of its period from the Unix epoch, so every
// replica computes the same instants. Registration belongs in package init
// functions: the scheduler starts with the process, and a registration after
// that panics.
//
// The job runs once per instant across the deployment: the replicas share
// the instant's lease through the primary database, the first to claim it
// runs the round, the others skip the instant, and a round still holding the
// lease keeps the next instants from everyone. On start-up the most recent
// instant no replica ran is caught up once — when the job has run before,
// the instant lies within the last day and no replica claimed it; see the
// package documentation for the rule in full. A job that must run on every
// replica registers with RegisterPerInstance instead.
//
// fn receives the context of the round it runs in. The context ends when the
// process begins shutting down or the round's lease is lost, so a long round
// can stop early, and a transaction opened on it refuses to run once the
// lease is gone; a round still running 5 seconds after its lease was lost
// fails the process — another replica may be running the job's next instant
// by then, and two rounds of a job never run at once. The context carries
// the round's identity — the job name and a trace id of the round's own, see
// execctx — and, with tracing on, the round's root span, so every statement,
// log line and span the job produces is annotated with the round and can be
// found again from any of them. An instant that passes while the previous
// round is still in flight is skipped.
//
// The framework opens a single connection to SQLite, so there a transaction
// of the job blocks the renewal of the round's lease: keep each transaction
// under 5 seconds — a longer one may hold the renewal back until the lease
// counts as lost, which ends the round; one over 10 seconds always does — or
// register work that only ever runs in one process with RegisterPerInstance.
//
// A registration that cannot be honored — no name, no schedule, a schedule
// that does not parse or names the Local zone, a name already taken, a nil
// fn — fails the process at startup.
func Register(fn func(ctx context.Context) error, spec string, name string) {
	register(fn, spec, name, false)
}

// RegisterPerInstance is Register for a job every replica runs on its own,
// without a lease: work that belongs to the process, such as refreshing a
// process-local cache or cleaning a local directory. It never catches up an
// instant. Everything else is as for Register.
func RegisterPerInstance(fn func(ctx context.Context) error, spec string, name string) {
	register(fn, spec, name, true)
}

// register is the body of Register and RegisterPerInstance.
func register(fn func(ctx context.Context) error, spec, name string, perInstance bool) {
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
	j.perInstance = perInstance
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
	if zone, named := zoneOf(spec); named && zone == "Local" {
		return nil, errors.Newf("cronjob %q: schedule %q names the Local zone, which is the process's own and need not be shared by every replica; name a fixed zone, or none for UTC", name, spec)
	}

	parsed, err := parser.Parse(spec)
	if err != nil {
		return nil, errors.Wrapf(err, "cronjob %q: invalid schedule %q", name, spec)
	}
	j := &job{name: name, spec: spec, fn: fn, schedule: inUTC(parsed)}
	if err := lease.ValidateName(j.leaseName()); err != nil {
		return nil, errors.Wrapf(err, "cronjob %q", name)
	}
	return j, nil
}

// zoneOf returns the zone a schedule names with its CRON_TZ= or TZ= prefix —
// the prefixes the parser reads — and whether it names one at all.
func zoneOf(spec string) (zone string, named bool) {
	first, _, _ := strings.Cut(spec, " ")
	if zone, named = strings.CutPrefix(first, "CRON_TZ="); named {
		return zone, true
	}
	return strings.CutPrefix(first, "TZ=")
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
		// stays. An expression naming Local itself never gets here — newJob
		// refuses it — so time.Local can only mean no zone was named.
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

// previousInstant returns the most recent instant of s at or before now,
// looking back catchUpLookback at most. The schedule only answers "the first
// instant after t", so the instant is found by bisection over t: the answer
// stays at or before now for every t before the instant and moves past now
// from the instant on.
func previousInstant(s cron.Schedule, now time.Time) (time.Time, bool) {
	lo, hi := now.Add(-catchUpLookback), now
	if first := s.Next(lo); first.IsZero() || first.After(now) {
		return time.Time{}, false
	}
	for hi.Sub(lo) > time.Millisecond {
		mid := lo.Add(hi.Sub(lo) / 2)
		if next := s.Next(mid); !next.IsZero() && !next.After(now) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return s.Next(lo), true
}

// start brings the scheduler up with every registered job, on a context
// derived from ctx. The context is the process context: its cancellation is
// the first sign of shutdown, and halts scheduling right then so no round
// starts while the process is on its way out; stop waits for the rounds
// already in flight. A registration that could not be honored fails the
// start, and so does a job under a lease on a primary database that cannot
// carry one.
func start(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()

	if errRegister != nil {
		return errRegister
	}
	if err := requireLeases(jobs); err != nil {
		return err
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

	s := newScheduler(jobs)
	s.start(ctx)
	current = s
	return nil
}

// requireLeases fails the start when a job runs under a lease and the
// primary database cannot carry one: the deployment could not coordinate,
// and every replica would run every instant as if it were alone.
func requireLeases(jobs []*job) error {
	for _, j := range jobs {
		if j.perInstance {
			continue
		}
		if err := lease.Available(); err != nil {
			return errors.Wrapf(err, "cronjob %q runs once per instant across the deployment, which needs a lease", j.name)
		}
		return nil
	}
	return nil
}

// stop ends the context of every loop and round in flight and waits for the
// rounds to return, for as long as ctx allows: a job that ignores its context
// cannot hold the shutdown hostage, and giving up on one is reported as the
// error bootstrap logs. Under bootstrap, scheduling has halted before stop
// runs: the process context ends the moment the drain begins, which stops
// the loops and ends the rounds' contexts; stop itself runs once the HTTP
// listener has drained and before the connections jobs may still be using
// are closed — without it, shutdown would kill jobs mid-write. In a process
// that never started the scheduler it is a no-op.
func stop(ctx context.Context) error {
	mu.Lock()
	s := current
	mu.Unlock()
	if s == nil {
		return nil
	}
	return s.stop(ctx)
}

// newScheduler builds a scheduler for jobs; start runs it.
func newScheduler(jobs []*job) *scheduler {
	return &scheduler{jobs: jobs, done: make(chan struct{})}
}

// start spawns a loop per job on a context derived from ctx.
func (s *scheduler) start(ctx context.Context) {
	loopCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	now := clk.Now()
	for _, j := range s.jobs {
		next := j.schedule.Next(now)
		log.Infoz("scheduled cronjob", zap.String("name", j.name), zap.String("spec", j.spec), zap.Bool("per_instance", j.perInstance), zap.Time("next", next))
		s.loops.Go(func() { j.loop(loopCtx, next) })
	}
	go func() {
		s.loops.Wait()
		close(s.done)
	}()
}

// stop ends the loops and waits for them, for as long as ctx allows.
func (s *scheduler) stop(ctx context.Context) error {
	s.cancel()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "gave up waiting for in-flight jobs")
	}
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
		log.Errorz("cronjob could not read its last instant", zap.Error(err), zap.String("name", j.name), zap.String("spec", j.spec))
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
// lease — its context ends with the lease, its transactions verify the lease
// first, and a round that will not stop once the lease is lost fails the
// process, see lease.Run — then gives the lease back so the next instant is
// free at once.
func (j *job) runInstant(ctx context.Context, at time.Time, catchUp bool) bool {
	if j.perInstance {
		// The round logs its own outcome.
		_ = j.run(ctx, at)
		return true
	}

	h, claimed, err := lease.ClaimSlot(ctx, j.leaseName(), at)
	if err != nil {
		log.Errorz("cronjob could not claim its instant", zap.Error(err), zap.String("name", j.name), zap.String("spec", j.spec), zap.Time("at", at))
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
	held, stopHold := lease.Hold(ctx, h)
	// The round logs its own outcome; Run's is the same error, already logged.
	_ = lease.Run(lease.WithHandle(held, h), j.leaseName(), func(ctx context.Context) error {
		return j.run(ctx, at, fields...)
	})
	lost := errors.Is(context.Cause(held), lease.ErrLost)
	stopHold()

	if lost {
		// The round outlived its lease — the renewals could not keep it, or
		// found it taken — and its context ended with it. The job's own error
		// says only that its context ended, so the loss is recorded here as
		// the round's outcome; the name is no longer this round's to give
		// back.
		log.Warnz("cronjob lost its lease during the round", zap.String("name", j.name), zap.String("spec", j.spec), zap.Time("at", at), zap.Uint64("term", h.Term()))
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
	defer func() { end(runErr) }()
	defer func() {
		if r := recover(); r != nil {
			runErr = util.PanicError(r)
			log.Errorz("cronjob panicked", append([]zap.Field{zap.Error(runErr)}, round...)...)
		}
	}()
	begin := time.Now()
	if runErr = j.fn(ctx); runErr != nil {
		log.Errorz("finished cronjob with error", append([]zap.Field{zap.Error(runErr)}, append(round, util.LogDuration(time.Since(begin)))...)...)
	} else {
		log.Infoz("finished cronjob", append(round, util.LogDuration(time.Since(begin)))...)
	}
	return runErr
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
