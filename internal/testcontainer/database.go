package testcontainer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	mysqlImage    = "mysql:8.4"
	mysqlPort     = "3306/tcp"
	mysqlDatabase = "test"
	mysqlUsername = "test"
	mysqlPassword = "test"

	postgresImage    = "postgres:17-alpine"
	postgresPort     = "5432/tcp"
	postgresDatabase = "test"
	postgresUsername = "test"
	postgresPassword = "test"

	sqliteDatabase = "test"
)

// SetupDatabase prepares the database dbType names and points the framework at
// it, returning the function that releases it. An empty dbType selects the
// framework default.
func SetupDatabase(dbType config.DBType) (func() error, error) {
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

// setupMySQL starts a mysql container of its own and points the framework at
// it. The returned function terminates that container.
func setupMySQL() (func() error, error) {
	muteContainerLog()
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

	host, port, err := containerEndpoint(ctx, c, mysqlPort)
	if err != nil {
		return nil, errors.CombineErrors(err, terminate())
	}

	applyConfigToEnv(config.MySQL{
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

// setupPostgres starts a postgres container of its own and points the
// framework at it. The returned function terminates that container.
func setupPostgres() (func() error, error) {
	muteContainerLog()
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

	host, port, err := containerEndpoint(ctx, c, postgresPort)
	if err != nil {
		return nil, errors.CombineErrors(err, terminate())
	}

	applyConfigToEnv(config.Postgres{
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

// setupSqlite prepares a file backed sqlite database in a directory of its own
// and points the framework at it. The returned function removes that directory.
//
// Sqlite needs no container, but a file still gives each call the isolation a
// container gives: the framework default is an in-memory database shared by
// every connection in the process.
func setupSqlite() (func() error, error) {
	dir, err := os.MkdirTemp("", "gst_sqlite_")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create sqlite directory")
	}
	path := filepath.Join(dir, sqliteDatabase+".db")

	applyConfigToEnv(config.Sqlite{
		Path:     path,
		Database: sqliteDatabase,
	})
	// File mode is the zero value of IsMemory and applyConfigToEnv skips zero
	// values, while the framework defaults the field to true.
	os.Setenv(config.SQLITE_IS_MEMORY, "false")
	useDatabase(config.DBSqlite)
	reportServiceReady(string(config.DBSqlite), path)

	return func() error { return os.RemoveAll(dir) }, nil
}

// useDatabase points the framework at dbType and turns on automatic table
// migration. database.auto_migrate defaults to false, and a freshly prepared
// database holds no tables at all, so a bootstrap needs it to create the ones
// its models declare.
func useDatabase(dbType config.DBType) {
	os.Setenv(config.DATABASE_TYPE, string(dbType))
	os.Setenv(config.DATABASE_AUTO_MIGRATE, "true")
}
