package testcontainer

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	postgresImage    = "postgres:17-alpine"
	postgresPort     = "5432/tcp"
	postgresDatabase = "test"
	postgresUsername = "test"
	postgresPassword = "test"
)

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

	host, port, err := endpoint(ctx, c, postgresPort)
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
