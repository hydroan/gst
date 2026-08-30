package testcontainer

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"
	// Registers the "mysql" driver the shared setup provisions databases with.
	_ "github.com/go-sql-driver/mysql"
	"github.com/hydroan/gst/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

const (
	mysqlImage = "mysql:8.4"
	mysqlPort  = "3306/tcp"

	// The dedicated setup runs as a plain user on the database the container
	// creates; the shared setup runs as root, which is what lets it create and
	// drop a database per test binary. Both use the same password.
	mysqlDatabase     = "test"
	mysqlUsername     = "test"
	mysqlRootUsername = "root"
	mysqlPassword     = "test"

	// mysqlSharedMaxConnections raises the server default of 151, which many
	// binaries sharing one instance would exhaust.
	mysqlSharedMaxConnections = 500
)

// mysqlSharedArgs is the command line the shared mysql container is created
// with. It is the single source for both the arguments themselves and the
// fingerprint sharedContainerName folds into the container name.
//
// Past the connection limit it turns off the durability machinery a throwaway
// instance has no use for. Creating the tables of every registered model is
// what a test binary spends most of its startup on, and every one of those DDL
// statements otherwise flushes the binary log and the redo log, twice per
// statement; the flushes, not the schema work, dominate. A test container that
// crashes is recreated rather than recovered, so the durability being traded
// away buys nothing to begin with.
//
// performance_schema stays on: it costs nothing measurable here and the sys
// views built on it are what a human debugging a shared instance reaches for.
var mysqlSharedArgs = []string{
	fmt.Sprintf("--max-connections=%d", mysqlSharedMaxConnections),
	"--skip-log-bin",
	"--innodb-flush-log-at-trx-commit=0",
	"--innodb-doublewrite=0",
}

// mysqlSharedDialect provisions per-binary databases inside the shared mysql
// container. DROP DATABASE cuts off live connections on its own here, no
// force clause needed.
var mysqlSharedDialect = sharedSQLDialect{
	driver: "mysql",
	adminDSN: func(host string, port uint) string {
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/?timeout=10s&readTimeout=30s",
			mysqlRootUsername, mysqlPassword, host, port)
	},
	listDatabases:  "SELECT schema_name FROM information_schema.schemata",
	createDatabase: func(name string) string { return "CREATE DATABASE `" + name + "`" },
	dropDatabase:   func(name string) string { return "DROP DATABASE `" + name + "`" },
}

// setupMySQL prepares a mysql database and points the framework at it. The
// returned functions release it and publish its schema template, see
// prepareSchemaTemplate.
func setupMySQL() (func() error, func(), error) {
	if dedicatedContainersRequested() {
		// A container of its own is shared with nobody, so there is no
		// template to copy from or to leave behind.
		release, err := setupDedicatedMySQL()
		return release, noSchemaTemplatePublish, err
	}
	return setupSharedMySQL()
}

// setupDedicatedMySQL starts a mysql container of its own and points the
// framework at it. The returned function terminates that container.
func setupDedicatedMySQL() (func() error, error) {
	prepareContainerRuntime()
	ctx := context.Background()

	c, err := mysql.Run(
		ctx, mysqlImage,
		mysql.WithDatabase(mysqlDatabase),
		mysql.WithUsername(mysqlUsername),
		mysql.WithPassword(mysqlPassword),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start mysql container")
	}
	terminate := func() error { return c.Terminate(ctx) }

	host, port, err := endpoint(ctx, c, mysqlPort)
	if err != nil {
		return nil, errors.CombineErrors(err, terminate())
	}

	ApplyConfigToEnv(config.MySQL{
		Host:     host,
		Port:     port,
		Database: mysqlDatabase,
		Username: mysqlUsername,
		Password: mysqlPassword,
	})
	useDatabase(config.DBMySQL)
	reportServiceReady(string(config.DBMySQL), fmt.Sprintf("%s:%d/%s", host, port, mysqlDatabase))

	return terminate, nil
}

// SetupStandaloneMySQL starts a mysql container of its own with the given
// database name and credentials, and returns its connection settings without
// touching the process environment or the framework's default database.
// Tests use it for topologies the single default database cannot express —
// a non-replicating stand-in replica for routing assertions, a second
// instance for cross-instance behavior. The returned function terminates
// the container.
func SetupStandaloneMySQL(database, username, password string) (config.MySQL, func() error, error) {
	prepareContainerRuntime()
	ctx := context.Background()

	options := []testcontainers.ContainerCustomizer{
		mysql.WithDatabase(database),
		mysql.WithPassword(password),
	}
	// The mysql module refuses to provision root as a plain user; root works
	// through the root password alone.
	if username != mysqlRootUsername {
		options = append(options, mysql.WithUsername(username))
	}
	c, err := mysql.Run(ctx, mysqlImage, options...)
	if err != nil {
		return config.MySQL{}, nil, errors.Wrap(err, "failed to start standalone mysql container")
	}
	terminate := func() error { return c.Terminate(ctx) }

	host, port, err := endpoint(ctx, c, mysqlPort)
	if err != nil {
		return config.MySQL{}, nil, errors.CombineErrors(err, terminate())
	}

	cfg := config.MySQL{
		Host:     host,
		Port:     port,
		Database: database,
		Username: username,
		Password: password,
		Charset:  "utf8mb4",
		Enabled:  true,
	}
	reportServiceReady("mysql-standalone", fmt.Sprintf("%s:%d/%s", host, port, database))
	return cfg, terminate, nil
}

// setupSharedMySQL attaches to the shared mysql container, creating it when
// it is not running yet, and provisions a database of its own for this test
// binary, filled from the shared schema template where one matches. The first
// returned function drops that database and the second publishes the template,
// see prepareSchemaTemplate; the container stays either way.
func setupSharedMySQL() (func() error, func(), error) {
	prepareContainerRuntime()
	ctx := context.Background()
	containerName := sharedContainerName(mysqlImage, mysqlSharedArgs...)

	var (
		host     string
		port     uint
		database string
		release  func() error
	)
	err := withSharedContainerLock(containerName, func() error {
		c, err := mysql.Run(
			ctx, mysqlImage,
			mysql.WithUsername(mysqlRootUsername),
			mysql.WithPassword(mysqlPassword),
			testcontainers.WithCmdArgs(mysqlSharedArgs...),
			testcontainers.WithReuseByName(containerName),
		)
		if err != nil {
			return errors.Wrap(err, "failed to start the shared mysql container")
		}

		if host, port, err = endpoint(ctx, c, mysqlPort); err != nil {
			return err
		}

		database, release, err = provisionSharedDatabase(ctx, mysqlSharedDialect, host, port)
		return err
	})
	if err != nil {
		return nil, nil, err
	}

	ApplyConfigToEnv(config.MySQL{
		Host:     host,
		Port:     port,
		Database: database,
		Username: mysqlRootUsername,
		Password: mysqlPassword,
	})
	useDatabase(config.DBMySQL)
	reportServiceReady(string(config.DBMySQL), fmt.Sprintf("%s:%d/%s", host, port, database))

	return release, prepareSchemaTemplate(host, port, database), nil
}
