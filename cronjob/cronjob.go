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
// another wall clock says so itself, with a CRON_TZ= prefix. The instants
// are read off each replica's own clock — the lease decides who runs one,
// on the database's clock — so the replicas' clocks must agree to within
// the schedule's granularity, as any clock-synchronized deployment's do: a
// replica running ahead claims an instant early, and the others find it
// taken when their clocks reach it.
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

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/lease"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/types"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"go.uber.org/zap"
)

var (
	mu sync.Mutex
	// jobs holds every registration, in registration order.
	jobs []*job
	// errRegister collects the registrations that could not be honored;
	// start reports them, so the process fails at startup instead of
	// silently running without those jobs.
	errRegister error
	// log is the scheduler's logger: the dedicated cronjob.log the lifecycle
	// binds before the component starts, or one writing to the global stream
	// in a process that never ran the lifecycle.
	log types.Logger
	// clk is the time the scheduler goes by. Tests swap in one they drive
	// by hand, so a schedule of hours plays out in microseconds.
	clk clock = systemClock{}

	// current is the started scheduler, nil until start. A registration
	// after that would never be scheduled, so it fails fast instead.
	current *scheduler
)

func init() {
	// Importing this package is what enables scheduling: through the
	// lifecycle registry, bootstrap starts the scheduler once the tables are
	// ready and stops it as soon as the process begins to drain. A project
	// that registers no job never imports the package and never runs a
	// scheduler.
	lifecycle.Register(lifecycle.Component{
		Name:      "cronjob",
		Stage:     lifecycle.StageComponent,
		SetLogger: setLogger,
		Start:     start,
		Stop:      stop,
	})
}

// setLogger binds the dedicated logger the lifecycle hands out.
func setLogger(l types.Logger) {
	mu.Lock()
	defer mu.Unlock()
	log = l
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
// can stop early, and a database.Transaction opened on it refuses to run
// once the lease is gone — a plain write is not checked, the context is
// what stops it; a round still running 5 seconds after its lease was lost
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
		// The lifecycle binds the dedicated logger before it starts the
		// component; a process that never ran the lifecycle (unit tests)
		// logs to the global stream. Opening cronjob.log here instead would
		// put a second rotation instance on the file once the lifecycle
		// opens its own.
		log = pkgzap.Fallback("cronjob")
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
