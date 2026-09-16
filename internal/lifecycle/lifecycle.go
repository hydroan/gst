// Package lifecycle is the registry of the framework components that have a
// lifetime of their own: clients of external systems, the scheduler, election
// loops, anything that owns a connection or a background goroutine — and,
// through the component package, a project's own long-running work. It also
// holds what such work has in common: Fail and FailNow, the ways out of a
// process that cannot go on, Await, the wait for work to return, and
// Interrupted, the test of whether work stopped because it was asked to.
//
// A component registers from its package initialiser, so importing its
// package is the single act that enables it: a project that never imports the
// package never links the component, never starts it and never pays for it.
// Bootstrap starts the providers — the clients — during its core phase, right
// after the backbone clients, so everything that runs after Bootstrap (a
// test harness seeding data, the routes-ready hooks Run fires first) can
// use them; it starts the components in Run, once every table they may
// touch exists and is seeded and right before the listener opens. All of
// them stop once the listener has drained, the components first and the
// providers after their last user. Within a stage the order is by name, so
// nothing in a stage may depend on another member of it. A component whose
// Enabled reports false is left out of the lifecycle entirely — no Start, no
// Stop — which makes "disabled means no-op" a bootstrap guarantee instead of
// a guard every component repeats.
package lifecycle

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/types"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// Stage is what a component is to the process, which decides when it starts
// and stops relative to the others.
type Stage int

const (
	// StageProvider is for clients of external systems. They start during
	// Bootstrap, right after the backbone clients, and stop last, after the
	// listener has drained and every user of theirs is gone.
	StageProvider Stage = iota
	// StageComponent is for work that runs alongside the server: the
	// scheduler, election loops, a project's own long-running work. They
	// start in Run, once the tables exist and right before the listener
	// opens, and stop first, right after the listener has drained.
	StageComponent
)

// stageCount is the number of stages; Register refuses any other value.
const stageCount = 2

// String names the stage the way errors and logs refer to it.
func (s Stage) String() string {
	switch s {
	case StageProvider:
		return "provider"
	case StageComponent:
		return "component"
	default:
		return fmt.Sprintf("Stage(%d)", int(s))
	}
}

// Component is one framework component with a lifetime of its own.
type Component struct {
	// Name uniquely identifies the component in the registry, in errors, in
	// logs and in the name of its log file. Framework components use their
	// package name (the final import path element); a project's work is
	// registered by the component package under its own name, "component".
	Name string

	// Stage is what the component is to the process; see Stage.
	Stage Stage

	// Enabled reports whether configuration enables the component. It is
	// called after configuration is loaded — registration happens in package
	// init functions, before any configuration exists, which is why this is
	// a function and not a value — and a component that reports false is
	// left out of the lifecycle. A nil Enabled means always enabled, so a
	// component without a configuration switch registers nothing extra.
	Enabled func() bool

	// SetLogger, when set, receives the dedicated logger writing <Name>.log
	// right before the component's stage starts, enabled or not: declaring
	// it is all a component does to log to its own file — it can neither
	// forget to create the logger nor misname the file. A disabled component
	// keeps the binding too: its file is created and stays empty, and code
	// logging through the package's logger while the component is off still
	// lands in the component's own file. Components without a dedicated log
	// file leave it nil; until the binding the package's logger keeps the
	// fallback the logging package installed, which routes entries to the
	// global sink.
	SetLogger func(types.Logger)

	// Start brings the component up and returns once it is running. It runs
	// only when Enabled reports true, so it needs no disabled guard of its
	// own.
	//
	// A component of StageComponent receives the process context — canceled
	// the moment shutdown begins, so a loop derived from it winds down on
	// its own — and keeps it for its lifetime. A provider's context bounds
	// the start itself (a dial, a handshake) and is canceled as soon as
	// Start returns: a provider serves until Stop, not until the process
	// begins to drain, so nothing it keeps may hang off the context.
	Start func(ctx context.Context) error

	// Stop halts the component and releases what it holds, waiting for its
	// in-flight work for as long as ctx allows. Optional; it runs at most
	// once, only after Start succeeded, and a returned error is logged here
	// so shutdown always continues.
	Stop func(ctx context.Context) error
}

// stageState is what the registry knows about one stage.
type stageState struct {
	// started is set by Start: a registration into the stage after that
	// would never start, so it fails fast instead.
	started bool
}

var (
	mu         sync.Mutex
	components []Component
	stages     [stageCount]stageState
	// running lists the components whose Start succeeded, across stages in
	// start order; Stop drains it in reverse.
	running []Component
)

// Register adds c to the registry. Registration happens in the component
// package's init function, so importing the package is what enables it.
//
// An empty name, a nil Start, an unknown stage, a duplicate name, or a
// registration after bootstrap has started the component's stage panics:
// each is a programmer error, and skipping it silently would drop a
// component the project compiled in on purpose.
func Register(c Component) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		panic("lifecycle: register requires a non-empty name")
	}
	if c.Start == nil {
		panic(fmt.Sprintf("lifecycle: register requires a non-nil Start for component %q", c.Name))
	}
	if c.Stage < 0 || c.Stage >= stageCount {
		panic(fmt.Sprintf("lifecycle: unknown stage %s for component %q", c.Stage, c.Name))
	}

	mu.Lock()
	defer mu.Unlock()

	if stages[c.Stage].started {
		panic(fmt.Sprintf("lifecycle: %s %q registered after bootstrap started the %s stage; register components in package init functions", c.Stage, c.Name, c.Stage))
	}
	if slices.ContainsFunc(components, func(r Component) bool { return r.Name == c.Name }) {
		panic(fmt.Sprintf("lifecycle: duplicate component registration for name %q", c.Name))
	}
	components = append(components, c)
}

// Components returns the registered components of stage sorted by name,
// enabled or not.
func Components(stage Stage) []Component {
	mu.Lock()
	defer mu.Unlock()

	return componentsOf(stage)
}

// componentsOf is Components for a caller that already holds mu.
func componentsOf(stage Stage) []Component {
	list := make([]Component, 0, len(components))
	for _, c := range components {
		if c.Stage == stage {
			list = append(list, c)
		}
	}
	slices.SortFunc(list, func(a, b Component) int { return strings.Compare(a.Name, b.Name) })
	return list
}

// Start binds the dedicated loggers of the stage's components, then starts
// the enabled ones in name order. It stops at the first component that fails
// to start and returns that failure; the components started before it keep
// running until Stop. Bootstrap calls it once per stage — the providers
// during Bootstrap, the components in Run once the tables are ready — and
// only once: a second call is an error, because starting a component twice
// would double its work. Business code never calls it.
func Start(ctx context.Context, stage Stage) error {
	mu.Lock()
	if stages[stage].started {
		mu.Unlock()
		return errors.Newf("lifecycle: %s stage already started", stage)
	}
	stages[stage].started = true
	pending := componentsOf(stage)
	mu.Unlock()

	// The bindings land before the first Start, for every registered
	// component of the stage: a compiled-in component always logs to its own
	// file.
	for _, c := range pending {
		if c.SetLogger != nil {
			c.SetLogger(pkgzap.New(c.Name + ".log"))
		}
	}

	for _, c := range pending {
		if c.Enabled != nil && !c.Enabled() {
			continue
		}
		if err := start(ctx, c); err != nil {
			return err
		}
		mu.Lock()
		running = append(running, c)
		mu.Unlock()
	}
	return nil
}

// start runs one component's Start on the context its stage promises.
func start(ctx context.Context, c Component) error {
	if c.Stage == StageProvider {
		// A provider's context ends with its Start, see Component.Start.
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	begin := time.Now()
	if err := c.Start(ctx); err != nil {
		return errors.Wrapf(err, "failed to start %s %q", c.Stage, c.Name)
	}
	zap.S().Debugw("component started", "stage", c.Stage.String(), "component", c.Name, util.LogDuration(time.Since(begin)))
	return nil
}

// Stop stops the started components in reverse start order — the components
// first, then the providers they used — giving each the remainder of ctx to
// finish its in-flight work. Once the process fails now the components are
// no longer waited for: each stops on a context that has ended, so it stops
// taking on work and reports what has not returned, and the rest of ctx goes
// to the providers; see FailNow. A Stop that fails is logged and the others
// still run. What has been stopped is not stopped again.
func Stop(ctx context.Context) {
	mu.Lock()
	stopping := running
	running = nil
	mu.Unlock()

	for _, c := range slices.Backward(stopping) {
		if c.Stop == nil {
			continue
		}
		if err := stopOne(ctx, c); err != nil {
			zap.S().Errorw("failed to stop component", "stage", c.Stage.String(), "component", c.Name, "err", err)
		}
	}
}

// stopOne runs one component's Stop: on ctx for a provider, and for a
// component on ctx ended the moment the process fails now.
func stopOne(ctx context.Context, c Component) error {
	if c.Stage == StageProvider {
		return c.Stop(ctx)
	}
	stopCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	failedNow := FailedNow()
	stopWatching := context.AfterFunc(failedNow, func() { cancel(context.Cause(failedNow)) })
	defer stopWatching()
	return c.Stop(stopCtx)
}

// Await waits for done to close for as long as ctx allows and reports
// whether it did. A done already closed wins over a ctx already ended, so
// work that has returned is never reported as given up on — as it would be
// by a select, which picks at random between cases that are both ready.
func Await(ctx context.Context, done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
	}
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

// failure is the context a component ends the process through, see Fail;
// failedNow is the one FailNow ends beside it.
var (
	failure, fail      = newFailure()
	failedNow, failNow = newFailure()
)

// newFailure builds the failure context and the function that ends it; a
// test rebuilds them to start from a process nothing has failed in.
func newFailure() (context.Context, context.CancelCauseFunc) {
	return context.WithCancelCause(context.Background())
}

// Fail reports a failure a component cannot recover from and the process
// cannot correctly go on with — a project's long-running work that ended
// before the process did. Bootstrap ends Run on the first one the way it
// ends on a listener failing: the process shuts down with err as the reason,
// and its orchestrator restarts it. Later failures change nothing; the first
// one is the reason.
func Fail(err error) {
	fail(orUnexplained(err))
}

// FailNow is Fail for a failure the shutdown must not wait on: work that
// will not stop once its lease is lost, running beside the replica that took
// the lease over. Every wait of a graceful shutdown — the drain delay, the
// requests in flight, the components' own work — would keep it running
// that much longer, so none is kept, from the call on: Stop no longer waits
// for the components, and bootstrap ends Run without the drain and bounds
// what is left of its teardown, the way controller-runtime drops its
// graceful shutdown when leader election is lost. It ends the process like
// Fail, with err as the reason unless an earlier failure already is.
func FailNow(err error) {
	err = orUnexplained(err)
	failNow(err)
	fail(err)
}

// orUnexplained returns err, or an error saying so when a component failed
// without one.
func orUnexplained(err error) error {
	if err == nil {
		return errors.New("lifecycle: a component failed without saying why")
	}
	return err
}

// Failure returns the context Fail and FailNow end, with the first failure
// as its cause.
func Failure() context.Context {
	return failure
}

// FailedNow returns the context FailNow ends, with its failure as the cause.
func FailedNow() context.Context {
	return failedNow
}
