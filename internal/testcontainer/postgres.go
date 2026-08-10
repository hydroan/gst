package testcontainer

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	// Registers the "pgx" driver the shared setup provisions databases with.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	postgresImage = "postgres:17-alpine"
	postgresPort  = "5432/tcp"

	// The dedicated setup runs on the database the container creates for its
	// bootstrap user; the shared setup runs as the postgres superuser, which
	// is what lets it create and drop a database per test binary. Both use the
	// same password.
	postgresDatabase      = "test"
	postgresUsername      = "test"
	postgresSuperUsername = "postgres"
	postgresPassword      = "test"

	// postgresSharedMaxConnections raises the server default of 100, which
	// many binaries sharing one instance would exhaust. Applied on first
	// creation only, see the shared containers comment in shared.go.
	postgresSharedMaxConnections = 500
)

// postgresSharedDialect provisions per-binary databases inside the shared
// postgres container. Unlike mysql, postgres refuses to drop a database with
// live connections, so the drop cuts them off with FORCE.
var postgresSharedDialect = sharedSQLDialect{
	driver: "pgx",
	adminDSN: func(host string, port uint) string {
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable&connect_timeout=10",
			postgresSuperUsername, postgresPassword, host, port, postgresSuperUsername)
	},
	listDatabases:  "SELECT datname FROM pg_database WHERE datistemplate = false",
	createDatabase: func(name string) string { return `CREATE DATABASE "` + name + `"` },
	dropDatabase:   func(name string) string { return `DROP DATABASE "` + name + `" WITH (FORCE)` },
}

// setupPostgres prepares a postgres database and points the framework at it.
// The returned function releases it.
func setupPostgres() (func() error, error) {
	if dedicatedContainersRequested() {
		return setupDedicatedPostgres()
	}
	return setupSharedPostgres()
}

// setupDedicatedPostgres starts a postgres container of its own and points
// the framework at it. The returned function terminates that container.
func setupDedicatedPostgres() (func() error, error) {
	prepareContainerRuntime()
	ctx := context.Background()

	c, err := postgres.Run(
		ctx, postgresImage,
		postgres.WithDatabase(postgresDatabase),
		postgres.WithUsername(postgresUsername),
		postgres.WithPassword(postgresPassword),
		// postgres restarts itself once during initdb, so it only counts as
		// ready after logging readiness twice and after the port is served.
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start postgres container")
	}
	terminate := func() error { return c.Terminate(ctx) }

	host, port, err := endpoint(ctx, c, postgresPort)
	if err != nil {
		return nil, errors.CombineErrors(err, terminate())
	}

	ApplyConfigToEnv(config.Postgres{
		Host:     host,
		Port:     port,
		Database: postgresDatabase,
		Username: postgresUsername,
		Password: postgresPassword,
		SSLMode:  "disable",
	})
	useDatabase(config.DBPostgres)
	reportServiceReady(string(config.DBPostgres), fmt.Sprintf("%s:%d/%s", host, port, postgresDatabase))

	return terminate, nil
}

// setupSharedPostgres attaches to the shared postgres container, creating it
// when it is not running yet, and provisions a database of its own for this
// test binary. The returned function drops that database; the container
// stays.
func setupSharedPostgres() (func() error, error) {
	prepareContainerRuntime()
	ctx := context.Background()
	containerName := sharedContainerName(postgresImage)

	var (
		host     string
		port     uint
		database string
		release  func() error
	)
	err := withSharedContainerLock(containerName, func() error {
		c, err := postgres.Run(
			ctx, postgresImage,
			postgres.WithPassword(postgresPassword),
			testcontainers.WithCmdArgs("-c", fmt.Sprintf("max_connections=%d", postgresSharedMaxConnections)),
			// postgres restarts itself once during initdb, so it only counts
			// as ready after logging readiness twice and after the port is
			// served.
			postgres.BasicWaitStrategies(),
			testcontainers.WithReuseByName(containerName),
		)
		if err != nil {
			return errors.Wrap(err, "failed to start the shared postgres container")
		}

		if host, port, err = endpoint(ctx, c, postgresPort); err != nil {
			return err
		}

		database, release, err = provisionSharedDatabase(ctx, postgresSharedDialect, host, port)
		return err
	})
	if err != nil {
		return nil, err
	}

	ApplyConfigToEnv(config.Postgres{
		Host:     host,
		Port:     port,
		Database: database,
		Username: postgresSuperUsername,
		Password: postgresPassword,
		SSLMode:  "disable",
	})
	useDatabase(config.DBPostgres)
	reportServiceReady(string(config.DBPostgres), fmt.Sprintf("%s:%d/%s", host, port, database))

	return release, nil
}
