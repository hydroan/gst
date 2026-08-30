package testutil

import (
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testcontainer"
)

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

	// Clickhouse prepares a clickhouse database and points the framework
	// provider at it, next to whatever Database selects: clickhouse is an
	// analytical instance beside the default database, never a replacement
	// for it. Tests reach it through config.App.Clickhouse or the provider.
	Clickhouse bool

	// Kafka prepares a kafka container and points the framework provider at
	// it. Tests reach it through config.App.Kafka or the provider.
	Kafka bool

	// Register registers the modules under test. It runs before the framework
	// bootstraps, which is where module registration belongs. Registration
	// mirrors the framework's Register style and reports nothing.
	Register func()

	// Routes registers routes that need a bootstrapped framework, such as the
	// generated router.Init of a project — which the field accepts directly:
	// Routes: router.Init. It runs after the bootstrap and before Seed; a
	// returned error fails the test setup.
	Routes func() error

	// Seed plants baseline rows such as a root account. It runs after Routes,
	// once the framework is up and the database is reachable, but before the
	// server starts serving, so no request can observe a half prepared state.
	// A returned error fails the test setup.
	Seed func() error
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
//			Routes:   router.Init,
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
	release, afterMigrate, err := s.prepare()
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
	// The bootstrap has migrated, so the schema is now what a later binary can
	// start from. Publishing here rather than earlier is what keeps a template
	// made of the tables the migration itself produced.
	afterMigrate()
	if s.Routes != nil {
		if err := s.Routes(); err != nil {
			panic(err)
		}
	}
	if s.Seed != nil {
		if err := s.Seed(); err != nil {
			panic(err)
		}
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
// returning the function that releases them and the one to run once the
// framework has migrated the database. The releases are collected as they
// succeed, so the returned function undoes whatever was already prepared even
// when a later step fails.
func (s Server) prepare() (release func(), afterMigrate func(), err error) {
	releases := make([]func(), 0, 5)
	release = func() {
		for _, done := range slices.Backward(releases) {
			done()
		}
	}

	// Every early return hands back a no-op hook: nothing was migrated, so
	// there is nothing to publish.
	afterMigrate = func() {}

	logDir, err := os.MkdirTemp("", "gst_logs_")
	if err != nil {
		return release, afterMigrate, errors.Wrap(err, "failed to create the test log directory")
	}
	releases = append(releases, func() {
		if removeErr := os.RemoveAll(logDir); removeErr != nil {
			reportReleaseFailure("log directory", removeErr)
		}
	})

	// A log directory of its own keeps the logs of a test run out of the
	// package source tree, where they would otherwise pile up next to the code.
	os.Setenv(config.LOGGER_DIR, logDir)
	listenOnFreePort()

	cleanDatabase, publishTemplate, err := testcontainer.SetupDatabase(s.Database)
	if err != nil {
		return release, afterMigrate, err
	}
	afterMigrate = publishTemplate
	releases = append(releases, func() {
		if releaseErr := cleanDatabase(); releaseErr != nil {
			reportReleaseFailure("database", releaseErr)
		}
	})

	if s.Redis {
		cleanCache, err := testcontainer.SetupRedis()
		if err != nil {
			return release, afterMigrate, err
		}
		releases = append(releases, func() {
			if releaseErr := cleanCache(); releaseErr != nil {
				reportReleaseFailure("cache", releaseErr)
			}
		})
	}

	if s.Clickhouse {
		cfg, cleanAnalytical, err := testcontainer.SetupClickhouse()
		if err != nil {
			return release, afterMigrate, err
		}
		releases = append(releases, func() {
			if releaseErr := cleanAnalytical(); releaseErr != nil {
				reportReleaseFailure("clickhouse", releaseErr)
			}
		})
		// SetupClickhouse hands the connection back instead of touching the
		// environment; exporting it is this server's decision, so the
		// bootstrap reads the prepared instance from the config like the
		// other services.
		testcontainer.ApplyConfigToEnv(cfg)
	}

	if s.Kafka {
		cleanBroker, err := testcontainer.SetupKafka()
		if err != nil {
			return release, afterMigrate, err
		}
		releases = append(releases, func() {
			if releaseErr := cleanBroker(); releaseErr != nil {
				reportReleaseFailure("kafka", releaseErr)
			}
		})
	}

	return release, afterMigrate, nil
}

// reportReleaseFailure reports that a prepared service could not be released.
// A release runs once the tests are over, so there is no test left to fail;
// what a leaked container needs is to be visible.
func reportReleaseFailure(name string, err error) {
	fmt.Fprintf(os.Stderr, "failed to release the test %s: %v\n", name, err)
}
