// Package testcontainer prepares the services the framework's tests run
// against, in containers started through testcontainers-go.
//
// What testcontainers-go provides is used as it is: a container started for
// one test binary alone is the library's to create, wait for and clean up,
// its reaper included, and nothing here duplicates any of that. Everything
// this package builds on top exists for one reason only, reuse: the mysql,
// postgres, redis, clickhouse and minio containers are shared by every test
// binary and outlive every run to keep the tests fast, which the library does
// not provide. Their fixed names, the isolation slot every binary claims
// inside them, the cleanup of slots whose binary is gone and keeping them out
// of the reaper's reach are this package's own for that reason alone. A
// container that is not reused takes none of it, and a need the library meets
// is met through the library, never through a mechanism of this package's own.
package testcontainer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gofrs/flock"
	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
)

// Shared containers.
//
// By default the mysql, postgres, redis, clickhouse and minio setups attach
// every test binary to one fixed-name container per image version instead of
// starting a container per package. The container is the shared layer;
// isolation moves one level down, where it is much cheaper: every binary gets
// a slot of its own inside the shared container, a database, a redis database
// index or a bucket. Releasing drops that slot but never terminates the
// container, so later runs start against a warm instance.
//
// The shared containers survive on purpose; removing one is a manual
// `docker rm -f <name>`. A tuning change in a container command line needs no
// such step: the arguments are part of the container name, so a changed
// command line names a container that does not exist yet and the previous one
// is simply left behind. See sharedContainerName.

// envDedicatedContainers switches every setup back to a dedicated container
// per test binary, terminated on release and watched by the reaper. It is the
// escape hatch when the shared layer cannot work: a remote docker daemon
// (liveness checks reach only local processes), a broken shared container, or
// debugging that needs a pristine instance.
const envDedicatedContainers = "GST_TEST_DEDICATED_CONTAINERS"

// dedicatedContainersRequested reports whether the escape hatch asks for a
// dedicated container per test binary. Any value strconv.ParseBool reads as
// true turns it on.
func dedicatedContainersRequested() bool {
	dedicated, err := strconv.ParseBool(os.Getenv(envDedicatedContainers))
	return err == nil && dedicated
}

// reuseAcrossRuns attaches to the shared container named name, creating it
// when it does not exist yet, and keeps it out of the reaper's reach.
//
// The reaper removes every container carrying the labels of its session once
// the test binaries of that session are gone, and testcontainers gives a
// reused container those labels like any other: the container would go with
// the run that created it, pulled from under every binary of another run
// still using it. Stripping the labels at creation is what lets the reaper
// stay on for everything else, so every dedicated container keeps its crash
// cleanup. The labels come from testcontainers.GenericLabels, the library's
// own account of what the reaper matches, not from a copy of their names.
func reuseAcrossRuns(name string) testcontainers.CustomizeRequestOption {
	return func(req *testcontainers.GenericContainerRequest) error {
		if err := testcontainers.WithReuseByName(name)(req); err != nil {
			return err
		}
		return testcontainers.WithConfigModifier(func(config *container.Config) {
			for key := range testcontainers.GenericLabels() {
				delete(config.Labels, key)
			}
		})(req)
	}
}

// sharedContainerName derives the fixed container name from an image
// reference and the command line the container is created with, e.g.
// "mysql:8.4" without arguments becomes "gst-test-mysql-8-4" and with
// arguments "gst-test-mysql-8-4-1f2e3d4c".
//
// Both inputs shape the name because reuse attaches to whatever container
// already carries it, while the creation options only shape a container that
// does not exist yet. A name blind to the arguments would keep serving an
// instance created by an older revision of this package, running a command
// line nothing in the source mentions any more and with no sign that it does.
// Folding the arguments in abandons that instance the way an image bump does,
// so a tuning change takes effect on its own instead of waiting for someone to
// remember to remove a container. Abandoned containers are removed by hand.
func sharedContainerName(image string, args ...string) string {
	return "gst-test-" + strings.Map(func(r rune) rune {
		switch r {
		case ':', '.', '/':
			return '-'
		}
		return r
	}, image) + sharedContainerArgsFingerprint(args)
}

// sharedContainerArgsFingerprint returns the suffix sharedContainerName
// carries for the command line arguments a container is created with. An
// empty command line has no fingerprint, so a container that takes no
// arguments keeps the plain image-derived name.
func sharedContainerArgsFingerprint(args []string) string {
	if len(args) == 0 {
		return ""
	}
	// The separator keeps two argument lists that concatenate to the same
	// text apart; it cannot occur inside an argument, those are C strings.
	sum := sha256.Sum256([]byte(strings.Join(args, "\x00")))
	return "-" + hex.EncodeToString(sum[:4])
}

// withSharedContainerLock runs fn while holding a file lock named after the
// shared container, serializing the ensure-and-claim sequence across
// concurrently starting test binaries: only one binary creates the container
// or claims an isolation slot at a time. testcontainers resolves a same-name
// creation race on its own, but the claim steps have no such arbiter.
//
// The lock blocks without a timeout: the holder may legitimately be pulling
// an image for minutes on a cold machine.
func withSharedContainerLock(containerName string, fn func() error) error {
	lock := flock.New(filepath.Join(os.TempDir(), containerName+".lock"))
	if err := lock.Lock(); err != nil {
		return errors.Wrapf(err, "failed to lock the shared container %s", containerName)
	}
	defer func() { _ = lock.Unlock() }()
	return fn()
}

// processAlive reports whether a pid names a running process, which is what
// decides whether an isolation slot still belongs to someone. The probe
// reaches local processes only, so sharing containers assumes the docker
// daemon runs on the machine the tests run on; against a remote daemon the
// escape hatch is the answer. A recycled pid keeps a slot claimed at worst
// until the impostor exits, which the stale age backstop then covers.
func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	// EPERM still proves a process: the signal bounced off another user's.
	return err == nil || errors.Is(err, syscall.EPERM)
}

// sharedDatabasePattern is the shape of a database a SQL setup provisions:
// the claiming pid, the creation time in unix seconds, then a random suffix
// that keeps two claims of a recycled pid apart.
var sharedDatabasePattern = regexp.MustCompile(`^gst_test_(\d+)_(\d+)_[0-9a-f]+$`)

// sharedDatabaseStaleAfter is how old an abandoned-looking database must be
// before the pid liveness verdict alone is overruled: a database this old
// whose pid is alive is a recycled pid, not a day-long test run.
const sharedDatabaseStaleAfter = 24 * time.Hour

// newSharedDatabaseName returns a database name matching
// sharedDatabasePattern.
func newSharedDatabaseName() string {
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	return fmt.Sprintf("gst_test_%d_%d_%s", os.Getpid(), time.Now().Unix(), hex.EncodeToString(suffix))
}

// sharedDatabaseAbandoned reports whether a database name marks a leftover
// whose owning binary is gone: its pid is dead, or the name is old enough
// that a live pid must be a recycled one. Names this setup did not generate
// are never abandoned, they are not ours to drop.
func sharedDatabaseAbandoned(name string) bool {
	match := sharedDatabasePattern.FindStringSubmatch(name)
	if match == nil {
		return false
	}
	pid, err := strconv.Atoi(match[1])
	if err != nil {
		return false
	}
	sec, err := strconv.ParseInt(match[2], 10, 64)
	if err != nil {
		return false
	}
	return !processAlive(pid) || time.Since(time.Unix(sec, 0)) > sharedDatabaseStaleAfter
}

// sharedSQLDialect describes how one SQL flavor provisions per-binary
// databases inside its shared container. It is what a SQL-backed service
// implements to take part in container sharing.
type sharedSQLDialect struct {
	// driver is the database/sql driver name the admin connection opens with.
	driver string
	// adminDSN connects to the shared container as a user allowed to create
	// and drop databases.
	adminDSN func(host string, port uint) string
	// listDatabases returns every database name of the instance; the caller
	// picks the abandoned ones.
	listDatabases string
	// createDatabase and dropDatabase quote the name in the flavor's own way;
	// dropDatabase must also cut off live connections where the flavor would
	// otherwise refuse to drop.
	createDatabase func(name string) string
	dropDatabase   func(name string) string
}

// provisionSharedDatabase creates the database of this test binary inside the
// shared container and returns the function that drops it again. On the way
// in it drops the abandoned databases of binaries that died without running
// their release, so leftovers survive one run at most. The caller holds the
// container lock, which is what makes the abandoned-or-live verdict and the
// concurrent claims safe.
func provisionSharedDatabase(ctx context.Context, d sharedSQLDialect, host string, port uint) (string, func() error, error) {
	admin, err := sql.Open(d.driver, d.adminDSN(host, port))
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to open the shared container admin connection")
	}

	if err := dropAbandonedSharedDatabases(ctx, admin, d); err != nil {
		return "", nil, errors.CombineErrors(err, admin.Close())
	}

	database := newSharedDatabaseName()
	if _, err := admin.ExecContext(ctx, d.createDatabase(database)); err != nil {
		return "", nil, errors.CombineErrors(
			errors.Wrapf(err, "failed to create the test database %s", database), admin.Close(),
		)
	}

	release := func() error {
		_, err := admin.ExecContext(ctx, d.dropDatabase(database))
		return errors.CombineErrors(
			errors.Wrapf(err, "failed to drop the test database %s", database), admin.Close(),
		)
	}
	return database, release, nil
}

// dropAbandonedSharedDatabases drops the test databases whose owning binary
// is gone, see sharedDatabaseAbandoned.
func dropAbandonedSharedDatabases(ctx context.Context, admin *sql.DB, d sharedSQLDialect) error {
	rows, err := admin.QueryContext(ctx, d.listDatabases)
	if err != nil {
		return errors.Wrap(err, "failed to list the databases of the shared container")
	}
	defer rows.Close()

	var abandoned []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return errors.Wrap(err, "failed to scan a database name")
		}
		if sharedDatabaseAbandoned(name) {
			abandoned = append(abandoned, name)
		}
	}
	if err := rows.Err(); err != nil {
		return errors.Wrap(err, "failed to list the databases of the shared container")
	}

	for _, name := range abandoned {
		if _, err := admin.ExecContext(ctx, d.dropDatabase(name)); err != nil {
			return errors.Wrapf(err, "failed to drop the abandoned test database %s", name)
		}
	}
	return nil
}
