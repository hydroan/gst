package testcontainer

import (
	"context"
	"fmt"

	// Registers the "clickhouse" driver the shared setup provisions databases
	// with.
	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/clickhouse"
)

const (
	clickhouseImage    = "clickhouse/clickhouse-server:24.8-alpine"
	clickhousePort     = "9000/tcp"
	clickhouseDatabase = "test"
	clickhouseUsername = "test"
	clickhousePassword = "test"
)

// clickhouseSharedDialect provisions per-binary databases inside the shared
// clickhouse container. The bootstrap user of the image holds every grant
// short of access management, which covers creating and dropping databases.
var clickhouseSharedDialect = sharedSQLDialect{
	driver: "clickhouse",
	adminDSN: func(host string, port uint) string {
		return fmt.Sprintf("clickhouse://%s:%s@%s:%d/default?dial_timeout=10s",
			clickhouseUsername, clickhousePassword, host, port)
	},
	listDatabases:  "SELECT name FROM system.databases",
	createDatabase: func(name string) string { return `CREATE DATABASE "` + name + `"` },
	dropDatabase:   func(name string) string { return `DROP DATABASE "` + name + `"` },
}

// SetupClickhouse prepares a clickhouse database and returns the
// configuration for building an application-held analytical instance with
// clickhouse.New, together with the function that releases it.
//
// Unlike SetupDatabase it never points the framework default database at the
// container: clickhouse is an analytical instance held next to the default
// database, not a replacement for it, so tests build their own handle and
// pass it to DatabaseOn and AggregateOn.
func SetupClickhouse() (config.Clickhouse, func() error, error) {
	if dedicatedContainersRequested() {
		return setupDedicatedClickhouse()
	}
	return setupSharedClickhouse()
}

// setupDedicatedClickhouse starts a clickhouse container of its own. The
// returned function terminates that container.
func setupDedicatedClickhouse() (config.Clickhouse, func() error, error) {
	prepareContainerRuntime()
	ctx := context.Background()

	c, err := clickhouse.Run(
		ctx, clickhouseImage,
		clickhouse.WithDatabase(clickhouseDatabase),
		clickhouse.WithUsername(clickhouseUsername),
		clickhouse.WithPassword(clickhousePassword),
	)
	if err != nil {
		return config.Clickhouse{}, nil, errors.Wrap(err, "failed to start clickhouse container")
	}
	terminate := func() error { return c.Terminate(ctx) }

	host, port, err := endpoint(ctx, c, clickhousePort)
	if err != nil {
		return config.Clickhouse{}, nil, errors.CombineErrors(err, terminate())
	}

	cfg := clickhouseConfig(host, port, clickhouseDatabase)
	reportServiceReady("clickhouse", fmt.Sprintf("%s:%d/%s", host, port, clickhouseDatabase))

	return cfg, terminate, nil
}

// setupSharedClickhouse attaches to the shared clickhouse container, creating
// it when it is not running yet, and provisions a database of its own for
// this test binary. The returned function drops that database; the container
// stays.
func setupSharedClickhouse() (config.Clickhouse, func() error, error) {
	prepareContainerRuntime()
	ctx := context.Background()
	containerName := sharedContainerName(clickhouseImage)

	var (
		host     string
		port     uint
		database string
		release  func() error
	)
	err := withSharedContainerLock(containerName, func() error {
		c, err := clickhouse.Run(
			ctx, clickhouseImage,
			clickhouse.WithUsername(clickhouseUsername),
			clickhouse.WithPassword(clickhousePassword),
			testcontainers.WithReuseByName(containerName),
		)
		if err != nil {
			return errors.Wrap(err, "failed to start the shared clickhouse container")
		}

		if host, port, err = endpoint(ctx, c, clickhousePort); err != nil {
			return err
		}

		database, release, err = provisionSharedDatabase(ctx, clickhouseSharedDialect, host, port)
		return err
	})
	if err != nil {
		return config.Clickhouse{}, nil, err
	}

	cfg := clickhouseConfig(host, port, database)
	reportServiceReady("clickhouse", fmt.Sprintf("%s:%d/%s", host, port, database))

	return cfg, release, nil
}

// clickhouseConfig assembles the configuration a test passes to
// clickhouse.New for the prepared database.
func clickhouseConfig(host string, port uint, database string) config.Clickhouse {
	return config.Clickhouse{
		Host:     host,
		Port:     port,
		Database: database,
		Username: clickhouseUsername,
		Password: clickhousePassword,
		// The timeout fields feed straight into the DSN, so they cannot stay
		// empty: the config defaults only apply through config.Init, which a
		// hand-built struct bypasses.
		DialTimeout: "10s",
		ReadTimeout: "30s",
		Enabled:     true,
	}
}
