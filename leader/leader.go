// Package leader runs, on one replica of a deployment at a time, the work a
// project registers: a loop that polls an external system, a relay that
// forwards an outbox, the one connection a protocol allows. Every replica
// campaigns for the name; the one that wins runs the work until it stops
// being the leader — the process shuts down, or the lease behind the
// leadership is lost — and the others keep campaigning, so the work moves to
// another replica within seconds of its leader dying.
//
// Importing the package is what enables the elections: the elector joins the
// process lifecycle from init, starts once the tables are ready and stops as
// soon as the process begins to drain; a project that registers no work
// never links it.
//
// Leadership is a lease on the primary database (see the lease package). The
// leader renews it every 5 seconds and gives itself up 10 seconds after the
// last renewal it managed, before the database lets another replica claim
// the name; a name whose leader crashed is free 15 seconds after that
// leader's last renewal and taken by the next campaign after that, 21
// seconds after the crash at most, and a name its leader released — the
// process shut down, the work returned — is taken by the next campaign at
// once, within 6 seconds. The work runs on a context that ends the moment
// the tenure does, and the transactions opened on that context end with
// it. Work that runs on past that point — it ignores its context — would
// run beside the new leader's, the very thing the lease exists to rule
// out, so 5 seconds after a tenure ended by a lost lease the process fails:
// bootstrap ends Run with the failure and the orchestrator restarts the
// replica. A ClickHouse primary database cannot carry leases, so a
// registration fails the start there.
//
// The work runs again from scratch on the replica that takes the name over,
// and its previous run may have been cut anywhere: what it must not repeat,
// it keeps in the database — the database the tenure's database.Transaction
// calls verify the lease against — and what it must resume, it finds there.
//
// A registration that cannot be honored — no name, nil work, a name already
// taken — fails the process at startup rather than dropping the work: the
// name is the work's identity in every log line about it and the name of its
// lease.
package leader

import (
	"context"
	"fmt"
	"math/rand/v2"
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
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

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

var (
	mu sync.Mutex
	// works holds every registration, in registration order.
	works []*work
	// errRegister collects the registrations that could not be honored;
	// start reports them, so the process fails at startup instead of
	// silently running without that work.
	errRegister error
	// log is the elector's logger: the dedicated leader.log the lifecycle
	// binds before the component starts, or one of the package's own in a
	// process that never ran the lifecycle.
	log types.Logger
	// current is the started elector, nil until start. A registration after
	// that would never campaign, so it fails fast instead.
	current *elector
)

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

// elector is one started elector: the work it campaigns for, the loops it
// campaigns in, and what ends them.
type elector struct {
	works []*work
	// cancel ends the context every loop runs on; stop calls it, and the
	// process context ending does the same.
	cancel context.CancelFunc
	loops  sync.WaitGroup
	// done closes once every loop has returned.
	done chan struct{}
}

func init() {
	// Importing this package is what enables the elections: through the
	// lifecycle registry, bootstrap starts the elector once the tables are
	// ready and stops it as soon as the process begins to drain. A project
	// that registers no work never imports the package and never campaigns.
	lifecycle.Register(lifecycle.Component{
		Name:      "leader",
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

// Register declares fn as the work named name, run on one replica of the
// deployment at a time. Registration belongs in package init functions: the
// elector starts with the process, and a registration after that panics.
//
// fn is expected to run until its context ends. The context ends when the
// process begins shutting down or the lease behind the leadership is lost —
// the replica could not renew it for 10 seconds, or found it taken — and
// whatever fn does after that must stop: a database.Transaction opened on
// the context refuses to run once the lease is gone, a plain write and a
// call already on the wire do not, so fn watches the context around its own
// side effects. fn that has not returned 5 seconds after its context ended
// by a lost lease fails the process, see the package documentation. The
// context carries the tenure's
// identity — the name and a trace id of the tenure's own, see execctx — so
// every statement and log line the work produces is annotated with the
// tenure and found again from any of them.
//
// fn that returns while still the leader — done, or failed — hands the name
// back: the campaign resumes after the campaign interval, on this replica or
// another, and fn runs again from scratch. A panic in fn is recovered and
// logged, and counts as a return with an error.
//
// The framework opens a single connection to SQLite, so there a transaction
// of fn blocks the renewal of the lease: keep each transaction under 5
// seconds — a longer one may hold the renewal back until the lease counts as
// lost, which ends the tenure; one over 10 seconds always does.
//
// A registration that cannot be honored — no name, a name already taken, a
// nil fn — fails the process at startup.
func Register(fn func(ctx context.Context) error, name string) {
	mu.Lock()
	defer mu.Unlock()

	if current != nil {
		panic(fmt.Sprintf("leader: %q registered after the elector started; register leader work in package init functions", name))
	}
	w, err := newWork(fn, name)
	if err != nil {
		errRegister = errors.Join(errRegister, err)
		return
	}
	works = append(works, w)
}

// newWork validates one registration. The caller holds mu: the duplicate
// check reads works.
func newWork(fn func(ctx context.Context) error, name string) (*work, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return nil, errors.New("leader: registered work has no name")
	case fn == nil:
		return nil, errors.Newf("leader %q: nil function", name)
	case slices.ContainsFunc(works, func(w *work) bool { return w.name == name }):
		return nil, errors.Newf("leader %q: registered twice", name)
	}
	w := &work{name: name, fn: fn}
	if err := lease.ValidateName(w.leaseName()); err != nil {
		return nil, errors.Wrapf(err, "leader %q", name)
	}
	return w, nil
}

// start brings the elector up for every registered work, on a context
// derived from ctx. The context is the process context: its cancellation is
// the first sign of shutdown, and ends every tenure and campaign right then;
// stop waits for the work to return. A registration that could not be
// honored fails the start, and so does any work at all on a primary database
// that cannot carry a lease.
func start(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()

	if errRegister != nil {
		return errRegister
	}
	if len(works) > 0 {
		if err := lease.Available(); err != nil {
			return errors.Wrapf(err, "leader %q runs on one replica of the deployment, which needs a lease", works[0].name)
		}
	}
	if log == nil {
		// The lifecycle binds the dedicated logger before it starts the
		// component; a process that never ran the lifecycle (unit tests)
		// logs to the global stream. Opening leader.log here instead would
		// put a second rotation instance on the file once the lifecycle
		// opens its own.
		log = pkgzap.Fallback("leader")
	}

	e := newElector(works)
	e.start(ctx)
	current = e
	return nil
}

// stop ends every tenure and campaign and waits for the work to return, for
// as long as ctx allows: work that ignores its context cannot hold the
// shutdown hostage, and giving up on it is reported as the error bootstrap
// logs. Under bootstrap the tenures have ended before stop runs: the process
// context ends the moment the drain begins; stop itself runs once the HTTP
// listener has drained and before the connections the work may still be
// using are closed. In a process that never started the elector it is a
// no-op.
func stop(ctx context.Context) error {
	mu.Lock()
	e := current
	mu.Unlock()
	if e == nil {
		return nil
	}
	return e.stop(ctx)
}

// newElector builds an elector for works; start runs it.
func newElector(works []*work) *elector {
	return &elector{works: works, done: make(chan struct{})}
}

// start spawns a campaign loop per work on a context derived from ctx.
func (e *elector) start(ctx context.Context) {
	loopCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	for _, w := range e.works {
		log.Infoz("campaigning for leadership", zap.String("name", w.name))
		e.loops.Go(func() { w.loop(loopCtx) })
	}
	go func() {
		e.loops.Wait()
		close(e.done)
	}()
}

// stop ends the loops and waits for them, for as long as ctx allows.
func (e *elector) stop(ctx context.Context) error {
	e.cancel()
	select {
	case <-e.done:
		return nil
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "gave up waiting for leader work to return")
	}
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

// lead runs the work under h until it returns or the tenure ends — the lease
// is lost, or ctx, the process, is shutting down — then gives the name back,
// unless the tenure ended because the name was no longer this holder's. The
// work runs on the tenure's context: it ends with the tenure, its
// database.Transaction calls verify the lease first, it carries the tenure's
// identity, and work that will not stop once the lease is lost fails the
// process, see lease.Run.
func (w *work) lead(ctx context.Context, h *lease.Handle) {
	held, stopHold := lease.Hold(ctx, h)
	traceID := util.TraceID()
	tenure := execctx.WithLeader(lease.WithHandle(held, h), w.name, traceID)
	fields := []zap.Field{zap.String("name", w.name), zap.Uint64("term", h.Term()), zap.String(consts.TRACE_ID, traceID)}
	log.Infoz("elected leader", fields...)

	begin := time.Now()
	err := lease.Run(tenure, w.leaseName(), w.run)
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
