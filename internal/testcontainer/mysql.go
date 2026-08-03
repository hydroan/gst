package testcontainer

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

const (
	mysqlImage    = "mysql:8.4"
	mysqlPort     = "3306/tcp"
	mysqlDatabase = "test"
	mysqlUsername = "test"
	mysqlPassword = "test"
)

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

	host, port, err := endpoint(ctx, c, mysqlPort)
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
