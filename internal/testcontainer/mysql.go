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
	// binaries sharing one instance would exhaust. Applied on first creation
	// only, see the shared containers comment in shared.go.
	mysqlSharedMaxConnections = 500
)

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
// returned function releases it.
func setupMySQL() (func() error, error) {
	if dedicatedContainersRequested() {
		return setupDedicatedMySQL()
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

// setupSharedMySQL attaches to the shared mysql container, creating it when
// it is not running yet, and provisions a database of its own for this test
// binary. The returned function drops that database; the container stays.
func setupSharedMySQL() (func() error, error) {
	prepareContainerRuntime()
	ctx := context.Background()
	containerName := sharedContainerName(mysqlImage)

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
			testcontainers.WithCmdArgs(fmt.Sprintf("--max-connections=%d", mysqlSharedMaxConnections)),
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
		return nil, err
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

	return release, nil
}
