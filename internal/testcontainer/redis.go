package testcontainer

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

const (
	redisImage = "redis:7-alpine"
	redisPort  = "6379/tcp"
)

// SetupRedis starts a redis container of its own and points the framework at
// it. The returned function terminates that container.
//
// A container of its own is what keeps one test package from seeing the keys of
// another, so tests reach for this rather than sharing an instance under
// separate namespaces.
func SetupRedis() (func() error, error) {
	muteContainerLog()
	ctx := context.Background()

	c, err := redis.Run(ctx, redisImage)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start redis container")
	}
	terminate := func() error { return c.Terminate(ctx) }

	host, port, err := containerEndpoint(ctx, c, redisPort)
	if err != nil {
		return nil, errors.CombineErrors(err, terminate())
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	// Only Addr is set: the framework reads Addrs in cluster mode alone, which
	// stays off by default.
	applyConfigToEnv(config.Redis{
		Addr:    addr,
		Enabled: true,
	})
	reportServiceReady("redis", addr)

	return terminate, nil
}
