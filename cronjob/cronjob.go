package cronjob

import (
	"context"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/logger"
	pkgzap "github.com/hydroan/gst/logger/zap"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var (
	c        *cron.Cron
	log      types.Logger
	cronjobs = make([]*cronjob, 0)
	parser   cron.Parser
	mu       sync.Mutex

	inited bool
)

type cronjob struct {
	name           string
	spec           string
	fn             func(ctx context.Context) error
	sched          cron.Schedule
	runImmediately bool
}

// Config defines the configuration for cronjob package
type Config struct {
	// RunImmediately indicates whether to run the cronjob immediately after registration
	// in addition to the scheduled execution
	RunImmediately bool `json:"run_immediately" yaml:"run_immediately" toml:"run_immediately"`
}

func init() {
	parser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
}

// stopTimeout bounds how long Stop waits for in-flight jobs, so a stuck job
// cannot hold the shutdown hostage.
var stopTimeout = 30 * time.Second

// Stop halts scheduling and waits for in-flight jobs to finish, bounded by
// stopTimeout. Bootstrap registers it into the shutdown sequence after the
// HTTP drain and before the connections jobs may still be using are closed;
// without it, shutdown would kill jobs mid-write. In a process that never
// ran Init it is a no-op.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if c == nil {
		return
	}

	select {
	case <-c.Stop().Done():
	case <-time.After(stopTimeout):
		log.Warnz("cronjob stop timed out waiting for in-flight jobs", zap.Duration("timeout", stopTimeout))
	}
}

func Init() (err error) {
	if log == nil {
		// Adopt the shared cronjob logger so this package never opens a
		// second lumberjack instance on the same file, which would race its
		// rotation. Bootstrap initializes logging before cronjob.Init, so
		// the local fallback only serves processes that never ran the
		// logging setup (e.g. unit tests).
		if log = logger.Cronjob; log == nil {
			log = pkgzap.New("cronjob.log")
		}
	}
	if c == nil {
		c = cron.New(cron.WithSeconds())
	}

	for _, cj := range cronjobs {
		register(cj)
	}

	c.Start()

	inited = true
	return nil
}

// Register cronjob can be called at any point before or after Init().
// The config parameter is optional and can be used to customize cronjob behavior.
//
// fn receives the context of the round it runs in. The context carries the
// round's identity — the job name and a trace id of the round's own, see
// execctx — and, with tracing on, the round's root span, so every statement,
// log line and span the job produces is annotated with the round and can be
// found again from any of them.
func Register(fn func(ctx context.Context) error, spec string, name string, config ...Config) {
	var cfg Config
	if len(config) > 0 {
		cfg = config[0]
	}

	mu.Lock()
	defer mu.Unlock()
	cj := &cronjob{
		name:           name,
		spec:           spec,
		fn:             fn,
		runImmediately: cfg.RunImmediately,
	}

	if inited {
		register(cj)
	} else {
		cronjobs = append(cronjobs, cj)
	}
}

func register(cj *cronjob) {
	if cj == nil {
		return
	}
	if cj.spec == "" {
		return
	}
	sched, err := parser.Parse(cj.spec)
	if err != nil {
		log.Errorz("failed to parse cronjob spec", zap.Error(err), zap.String("name", cj.name), zap.String("spec", cj.spec))
		return
	}
	cj.sched = sched

	// run executes one round. Round identity, panic recovery, timing and
	// outcome logging live here so the immediate run and every scheduled run
	// share a single code path; runErr stays local to the round so concurrent
	// rounds of different jobs never share error state.
	//
	// A failure goes out as a typed error field, never formatted into the
	// message: the logging layer derives error_stack from that field, and for
	// a job this entry is the only record of the failure, so it has to
	// locate the failing line and not just name the job. Every outcome entry
	// carries the round's trace id — the id the round's statements and log
	// lines carry too — so the round is found again from any of them.
	run := func() {
		ctx, traceID, end := beginRound(cj.name)
		var runErr error
		// Registered before the recovery below so that it runs after it: a
		// panic is recorded on the round's span as its outcome.
		defer func() { end(runErr) }()
		defer func() {
			if r := recover(); r != nil {
				runErr = panicError(r)
				log.Errorz("cronjob panicked", zap.Error(runErr), zap.String("name", cj.name), zap.String("spec", cj.spec), zap.String(consts.TRACE_ID, traceID))
			}
		}()
		begin := time.Now()
		if runErr = cj.fn(ctx); runErr != nil {
			log.Errorz("finished cronjob with error", zap.Error(runErr), zap.String("name", cj.name), zap.String("spec", cj.spec), zap.String(consts.TRACE_ID, traceID), zap.Time("next", cj.sched.Next(begin)), util.LogDuration(time.Since(begin)))
		} else {
			log.Infoz("finished cronjob", zap.String("name", cj.name), zap.String("spec", cj.spec), zap.String(consts.TRACE_ID, traceID), zap.Time("next", cj.sched.Next(begin)), util.LogDuration(time.Since(begin)))
		}
	}
	// SkipIfStillRunning drops a tick while the previous round is still in
	// flight: a slow round must never have new rounds piled on top of it,
	// multiplying downstream calls. The immediate run goes through the same
	// wrapped job, so it holds the same in-flight guard as scheduled rounds;
	// if it overlaps the first tick the two collapse into one round, which is
	// exactly the "run once as soon as possible" the flag promises.
	job := cron.NewChain(cron.SkipIfStillRunning(cronLogger{name: cj.name})).Then(cron.FuncJob(run))

	if cj.runImmediately {
		go job.Run()
	}

	if _, addErr := c.AddJob(cj.spec, job); addErr != nil {
		log.Errorz("failed to add cronjob", zap.Error(addErr), zap.String("name", cj.name), zap.String("spec", cj.spec))
	} else {
		log.Infoz("successfully add cronjob", zap.String("name", cj.name), zap.String("spec", cj.spec), zap.Bool("run_immediately", cj.runImmediately))
	}
}

// beginRound opens one round of the named job and returns the context the job
// runs on, the round's trace id, and the function that closes the round with
// its outcome.
//
// The context carries the round's identity — the job name and the trace id —
// for everything the job does downstream: statement comments, the SQL log and
// the business log annotate themselves with it, the way they do with a
// request's. With tracing on the round also gets a root span, the parent of
// every span the job's operations open, and the trace id is that span's; with
// tracing off the id is generated, the way the request middleware generates
// one.
func beginRound(name string) (ctx context.Context, traceID string, end func(err error)) {
	ctx = context.Background()
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

// cronLogger adapts the package logger to cron.Logger for chain wrappers such
// as SkipIfStillRunning, tagging every entry with the job name the wrapper
// itself does not know — a bare "skip" line would leave no way to tell which
// job was dropped.
type cronLogger struct {
	name string
}

func (l cronLogger) Info(msg string, keysAndValues ...any) {
	log.Infow("cronjob "+msg, append([]any{"name", l.name}, keysAndValues...)...)
}

func (l cronLogger) Error(err error, msg string, keysAndValues ...any) {
	log.Errorw("cronjob "+msg, append([]any{"name", l.name, "error", err}, keysAndValues...)...)
}
