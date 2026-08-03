package testutil

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
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

// SetupMySQL starts a mysql container of its own and points the framework at
// it. The returned function terminates that container.
func SetupMySQL() (func() error, error) {
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

	ApplyConfigToEnv(config.MySQL{
		Host:     host,
		Port:     port,
		Database: mysqlDatabase,
		Username: mysqlUsername,
		Password: mysqlPassword,
	})
	useDatabase(config.DBMySQL)
	reportDatabaseReady(config.DBMySQL, fmt.Sprintf("%s:%d/%s", host, port, mysqlDatabase))

	return terminate, nil
}

// SetupPostgres starts a postgres container of its own and points the
// framework at it. The returned function terminates that container.
func SetupPostgres() (func() error, error) {
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

	ApplyConfigToEnv(config.Postgres{
		Host:     host,
		Port:     port,
		Database: postgresDatabase,
		Username: postgresUsername,
		Password: postgresPassword,
		SSLMode:  "disable",
	})
	useDatabase(config.DBPostgres)
	reportDatabaseReady(config.DBPostgres, fmt.Sprintf("%s:%d/%s", host, port, postgresDatabase))

	return terminate, nil
}

// SetupSqlite prepares a file backed sqlite database in a directory of its own
// and points the framework at it. The returned function removes that directory.
//
// Sqlite needs no container, but a file still gives each call the isolation a
// container gives: the framework default is an in-memory database shared by
// every connection in the process.
func SetupSqlite() (func() error, error) {
	dir, err := os.MkdirTemp("", "gst_sqlite_")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create sqlite directory")
	}
	path := filepath.Join(dir, sqliteDatabase+".db")

	ApplyConfigToEnv(config.Sqlite{
		Path:     path,
		Database: sqliteDatabase,
	})
	// File mode is the zero value of IsMemory and ApplyConfigToEnv skips zero
	// values, while the framework defaults the field to true.
	os.Setenv(config.SQLITE_IS_MEMORY, "false")
	useDatabase(config.DBSqlite)
	reportDatabaseReady(config.DBSqlite, path)

	return func() error { return os.RemoveAll(dir) }, nil
}

// containerEndpoint returns the host and the published port c maps port to.
func containerEndpoint(ctx context.Context, c testcontainers.Container, port string) (string, uint, error) {
	host, err := c.Host(ctx)
	if err != nil {
		return "", 0, errors.Wrapf(err, "failed to resolve container host for port %s", port)
	}
	mapped, err := c.MappedPort(ctx, port)
	if err != nil {
		return "", 0, errors.Wrapf(err, "failed to resolve mapped port for port %s", port)
	}
	return host, uint(mapped.Num()), nil
}

// useDatabase points the framework at dbType and turns on automatic table
// migration, so a bootstrap creates its tables in the freshly prepared
// database.
func useDatabase(dbType config.DBType) {
	os.Setenv(config.DATABASE_TYPE, string(dbType))
	EnableAutoMigrate()
}

// reportDatabaseReady prints where the prepared database lives. Container
// logging is muted, see muteContainerLog, so this is the only line a failing
// test leaves behind to reconnect by hand.
func reportDatabaseReady(dbType config.DBType, target string) {
	fmt.Fprintf(os.Stdout, "test database ready: %s %s\n", dbType, target)
}

var muteContainerLogOnce sync.Once

// muteContainerLog silences the logging testcontainers does on its own. Its
// default logger writes to stderr as soon as the test binary runs with -v,
// which buries the output of the test itself under image pull, reaper and
// container lifecycle noise. Each Setup function reports the one line that
// matters instead, see reportDatabaseReady.
func muteContainerLog() {
	muteContainerLogOnce.Do(func() {
		tclog.SetDefault(log.New(io.Discard, "", 0))
	})
}
