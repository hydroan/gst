// Package lock runs a piece of work once at a time across a deployment: an
// administrator's "rebuild the report", a refresh of a credential every
// replica shares, anything that must not run twice at once and is done when
// it returns. A lock is declared once, in a package variable, and tried from
// wherever the work is triggered. The try never waits: a request blocked on
// a lock held elsewhere would tie up a connection and a worker for as long
// as the other holder runs, so a held lock is refused at once, with ErrHeld,
// and the caller answers accordingly — a conflict to the client, a skipped
// run to the log.
//
// Importing the package is what enables locks: the declared locks join the
// process lifecycle from init, and startup checks them — every name one the
// lease table can hold, no name declared twice, a primary database that can
// carry a lease — so a lock that could never be taken fails the process
// instead of failing its first try. A project that declares no lock never
// links it.
//
// A lock is a lease on the primary database (see the lease package): TryRun
// claims the name, keeps it renewed while the work runs, and gives it back
// once the work has returned. The work runs on a context that ends when the
// lease is lost — the replica could not renew it for 10 seconds, or found it
// taken — or the caller's context ends, and the database.Transaction calls
// it makes verify the lease first. Work cut short by a lost lease is
// reported as ErrLost even when it returned nothing: another holder may
// have started the same work since, so its result is not the whole story.
// Work that runs on past that point — it ignores its context — would run
// beside the new holder's, so 5 seconds after the loss the process fails,
// as it does for a leader or a cron round. A ClickHouse primary database
// cannot carry leases, so a declared lock fails the start there.
//
// The framework opens a single connection to SQLite, so there a transaction
// of the work blocks the renewal of the lease: keep each transaction under 5
// seconds — a longer one may hold the renewal back until the lease counts as
// lost, which ends the work; one over 10 seconds always does.
package lock

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/lease"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/types"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"go.uber.org/zap"
)

// releaseTimeout bounds the statement that gives the name back once the work
// has returned.
const releaseTimeout = 5 * time.Second

var (
	// ErrHeld reports a lock held elsewhere — the work is running on another
	// replica, or in another request of this one — which TryRun never waits
	// for.
	ErrHeld = errors.New("lock held elsewhere")
	// ErrLost reports work cut short because the lease behind the lock was
	// lost while it ran; a transaction that refused to run for the same
	// reason reports it too.
	ErrLost = lease.ErrLost
)

var (
	mu sync.Mutex
	// locks holds every declaration, in declaration order.
	locks []*Lock
	// errDeclare collects the declarations that could not be honored; start
	// reports them, so the process fails at startup instead of a lock
	// failing its first try.
	errDeclare error
	// log is the package's logger: the dedicated lock.log the lifecycle
	// binds before the component starts, or the global log stream until
	// then; see logger.
	log types.Logger
	// started is set by start. A declaration after that was never checked,
	// so it fails fast instead.
	started bool
)

// Lock is one declared lock.
type Lock struct {
	name string
}

// Name returns the lock's name.
func (l *Lock) Name() string {
	return l.name
}

// leaseName is the coordinated name the lock is claimed under.
func (l *Lock) leaseName() string {
	return "lock:" + l.name
}

func init() {
	// Importing this package is what enables locks: through the lifecycle
	// registry, bootstrap checks the declared locks once the tables are
	// ready. A project that declares no lock never imports the package.
	lifecycle.Register(lifecycle.Component{
		Name:      "lock",
		Stage:     lifecycle.StageComponent,
		SetLogger: setLogger,
		Start:     start,
	})
}

// setLogger binds the dedicated logger the lifecycle hands out.
func setLogger(l types.Logger) {
	mu.Lock()
	defer mu.Unlock()
	log = l
}

// logger returns the package's logger: the bound lock.log, or, until the
// lifecycle binds it — a try made during Bootstrap, a unit test — a logger
// that writes to the global log stream. Opening lock.log here instead would
// put a second rotation instance on the file once the lifecycle opens its
// own.
func logger() types.Logger {
	mu.Lock()
	defer mu.Unlock()
	if log == nil {
		log = pkgzap.Fallback("lock")
	}
	return log
}

// New declares the lock named name and returns it. Declarations belong in
// package variables: the locks are checked as the process starts, and a
// declaration after that panics. A declaration that cannot be honored — no
// name, a name already declared, a name the lease table cannot hold — fails
// the process at startup.
func New(name string) *Lock {
	mu.Lock()
	defer mu.Unlock()

	if started {
		panic(fmt.Sprintf("lock: %q declared after the locks were checked; declare locks in package variables", name))
	}
	name = strings.TrimSpace(name)
	l := &Lock{name: name}
	// A declaration that cannot be honored is recorded for the start to
	// report and stays out of locks, so that it neither shadows a later
	// declaration of the name nor counts as one.
	switch {
	case name == "":
		errDeclare = errors.Join(errDeclare, errors.New("lock: declared lock has no name"))
		return l
	case slices.ContainsFunc(locks, func(other *Lock) bool { return other.name == name }):
		errDeclare = errors.Join(errDeclare, errors.Newf("lock %q: declared twice", name))
		return l
	}
	if err := lease.ValidateName(l.leaseName()); err != nil {
		errDeclare = errors.Join(errDeclare, errors.Wrapf(err, "lock %q", name))
		return l
	}
	locks = append(locks, l)
	return l
}

// start checks the declarations: one that could not be honored fails the
// start, and so does any lock at all on a primary database that cannot carry
// a lease.
func start(context.Context) error {
	mu.Lock()
	err := errDeclare
	declared := slices.Clone(locks)
	if err == nil && len(declared) > 0 {
		if err = lease.Available(); err != nil {
			err = errors.Wrapf(err, "lock %q runs its work once at a time across the deployment, which needs a lease", declared[0].name)
		}
	}
	if err == nil {
		started = true
	}
	mu.Unlock()
	if err != nil {
		return err
	}

	for _, l := range declared {
		logger().Infoz("declared lock", zap.String("name", l.name))
	}
	return nil
}

// TryRun runs fn under the lock, once, without waiting: ErrHeld when the lock
// is held elsewhere, else what fn returned — or ErrLost, joined with fn's
// error if any, when the lease behind the lock was lost while fn ran. fn
// receives a context that ends when the lease is lost or ctx ends, carries
// the lease so that database.Transaction calls on it verify it first, and
// keeps whatever identity ctx carried: the request's, the round's. A panic
// in fn is recovered into an error carrying its stack. fn that has not
// returned 5 seconds after its context ended by a lost lease fails the
// process, see the package documentation.
func (l *Lock) TryRun(ctx context.Context, fn func(ctx context.Context) error) error {
	h, won, err := lease.Claim(ctx, l.leaseName())
	if err != nil {
		return errors.Wrapf(err, "lock %q", l.name)
	}
	if !won {
		return errors.Wrapf(ErrHeld, "lock %q", l.name)
	}

	held, stopHold := lease.Hold(ctx, h, logger())
	err = lease.Run(lease.WithHandle(held, h), h, logger(), fn)
	lost := errors.Is(context.Cause(held), lease.ErrLost)
	stopHold()
	if lost {
		// The name is no longer this holder's to give back, and the work's
		// result is not the whole story: another holder may have started the
		// same work since.
		return errors.Join(errors.Wrapf(ErrLost, "lock %q", l.name), err)
	}

	// The release outlives ctx on purpose: at shutdown ctx is already gone,
	// and the name must still be handed back so the next try need not wait
	// the lease out.
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()
	if releaseErr := h.Release(releaseCtx); releaseErr != nil {
		logger().Warnz("lock could not release its lease", zap.Error(releaseErr), zap.String("name", l.name))
	}
	return err
}
