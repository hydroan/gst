package testutil

import (
	"os"
	"slices"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/config"
)

// Token authenticates every request a test makes. Run installs it as the
// never-expiring token, so a test client only has to pass it along.
const Token = "-"

// Server declares what a test package needs before its tests can run. Every
// field is optional: the zero value prepares the framework defaults, which is
// an sqlite database and no cache, and registers nothing.
type Server struct {
	// Database selects which database the tests run against. It defaults to
	// config.DBSqlite, the framework default, which needs no container.
	Database config.DBType

	// Redis prepares a redis container and points the framework at it. Modules
	// that keep sessions or cache entries need it.
	Redis bool

	// Register registers the modules under test. It runs before the framework
	// bootstraps, which is where module registration belongs.
	Register func()

	// Seed runs once the framework is up and the database is reachable, but
	// before the server starts serving, so no request can observe a half
	// prepared state. Baseline rows such as a root account belong here, as
	// does anything else that needs a bootstrapped framework, route
	// registration included.
	Seed func()
}

// Run prepares what s declares, starts the test server, runs the tests and
// releases everything afterwards. It is the whole body of a test package's
// TestMain:
//
//	func TestMain(m *testing.M) {
//		testutil.Run(m, testutil.Server{
//			Database: config.DBMySQL,
//			Redis:    true,
//			Register: func() { iam.Register() },
//		})
//	}
//
// Run does not return: it exits the test binary with the result of the tests.
func Run(m *testing.M, s Server) {
	os.Exit(run(m, s))
}

// run holds the body of Run so that the deferred releases still happen: the
// os.Exit in Run would skip them.
func run(m *testing.M, s Server) int {
	release, err := s.prepare()
	defer release()
	if err != nil {
		panic(err)
	}

	if s.Register != nil {
		s.Register()
	}
	if err := bootstrap.Bootstrap(); err != nil {
		panic(err)
	}
	if s.Seed != nil {
		s.Seed()
	}

	go func() {
		if err := bootstrap.Run(); err != nil {
			panic(err)
		}
	}()
	mustWaitForServer()

	return m.Run()
}

// prepare sets up the backing services and the shared test configuration,
// returning the function that releases them. The releases are collected as
// they succeed, so the returned function undoes whatever was already prepared
// even when a later step fails.
func (s Server) prepare() (release func(), err error) {
	releases := make([]func(), 0, 3)
	release = func() {
		for _, done := range slices.Backward(releases) {
			done()
		}
	}

	logDir, err := os.MkdirTemp("", "gst_logs_")
	if err != nil {
		return release, errors.Wrap(err, "failed to create the test log directory")
	}
	releases = append(releases, func() {
		if removeErr := os.RemoveAll(logDir); removeErr != nil {
			reportReleaseFailure("log directory", removeErr)
		}
	})

	// A log directory of its own keeps the logs of a test run out of the
	// package source tree, where they would otherwise pile up next to the code.
	os.Setenv(config.LOGGER_DIR, logDir)
	os.Setenv(config.AUTH_NONE_EXPIRE_TOKEN, Token)
	listenOnFreePort()

	cleanDatabase, err := setupDatabase(s.Database)
	if err != nil {
		return release, err
	}
	releases = append(releases, func() {
		if releaseErr := cleanDatabase(); releaseErr != nil {
			reportReleaseFailure("database", releaseErr)
		}
	})

	if s.Redis {
		cleanCache, err := setupRedis()
		if err != nil {
			return release, err
		}
		releases = append(releases, func() {
			if releaseErr := cleanCache(); releaseErr != nil {
				reportReleaseFailure("cache", releaseErr)
			}
		})
	}

	return release, nil
}

// setupDatabase prepares the database dbType names. An empty dbType selects
// the framework default.
func setupDatabase(dbType config.DBType) (func() error, error) {
	if len(dbType) == 0 {
		dbType = config.DBSqlite
	}

	switch dbType {
	case config.DBSqlite:
		return setupSqlite()
	case config.DBMySQL:
		return setupMySQL()
	case config.DBPostgres:
		return setupPostgres()
	default:
		return nil, errors.Newf("no test database available for %q, supported are %q, %q and %q",
			dbType, config.DBSqlite, config.DBMySQL, config.DBPostgres)
	}
}
