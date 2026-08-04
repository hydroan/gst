package testcontainer

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/testcontainers/testcontainers-go/modules/clickhouse"
)

const (
	clickhouseImage    = "clickhouse/clickhouse-server:24.8-alpine"
	clickhousePort     = "9000/tcp"
	clickhouseDatabase = "test"
	clickhouseUsername = "test"
	clickhousePassword = "test"
)

// SetupClickhouse starts a clickhouse container and returns the configuration
// for building an application-held analytical instance with clickhouse.New,
// together with the function that terminates the container.
//
// Unlike SetupDatabase it never points the framework default database at the
// container: clickhouse is an analytical instance held next to the default
// database, not a replacement for it, so tests build their own handle and
// pass it to DatabaseOn and AggregateOn.
func SetupClickhouse() (config.Clickhouse, func() error, error) {
	muteContainerLog()
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

	cfg := config.Clickhouse{
		Host:     host,
		Port:     port,
		Database: clickhouseDatabase,
		Username: clickhouseUsername,
		Password: clickhousePassword,
		// The timeout fields feed straight into the DSN, so they cannot stay
		// empty: the config defaults only apply through config.Init, which a
		// hand-built struct bypasses.
		DialTimeout: "10s",
		ReadTimeout: "30s",
		Enabled:     true,
	}
	reportServiceReady("clickhouse", fmt.Sprintf("%s:%d/%s", host, port, clickhouseDatabase))

	return cfg, terminate, nil
}
