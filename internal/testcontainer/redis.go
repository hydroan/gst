package testcontainer

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

const (
	redisImage = "redis:7-alpine"
	redisPort  = "6379/tcp"

	// redisSharedDatabases raises the server default of 16: the shared
	// container hands every test binary a database index of its own, and the
	// indexes 1 through 255 are the isolation slots. Index 0 stays unclaimed,
	// it is where a hand-typed debugging connection lands. Applied on first
	// creation only, see the shared containers comment in shared.go.
	redisSharedDatabases = 256

	// redisLeaseKey marks a claimed database index. Its value names the
	// claiming process, see formatRedisLease.
	redisLeaseKey = "gst:test:lease"
)

// SetupRedis prepares a redis database and points the framework at it,
// returning the function that releases it. Modules that keep sessions or
// cache entries need it.
func SetupRedis() (func() error, error) {
	if dedicatedContainersRequested() {
		return setupDedicatedRedis()
	}
	return setupSharedRedis()
}

// setupDedicatedRedis starts a redis container of its own and points the
// framework at it. The returned function terminates that container.
func setupDedicatedRedis() (func() error, error) {
	prepareContainerRuntime()
	ctx := context.Background()

	c, err := redis.Run(ctx, redisImage)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start redis container")
	}
	terminate := func() error { return c.Terminate(ctx) }

	host, port, err := endpoint(ctx, c, redisPort)
	if err != nil {
		return nil, errors.CombineErrors(err, terminate())
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	// Only Addr is set: the framework reads Addrs in cluster mode alone, which
	// stays off by default.
	ApplyConfigToEnv(config.Redis{
		Addr:    addr,
		Enabled: true,
	})
	reportServiceReady("redis", addr)

	return terminate, nil
}

// setupSharedRedis attaches to the shared redis container, creating it when
// it is not running yet, and claims a database index of its own for this test
// binary. The returned function flushes that database, which also clears its
// lease; the container stays.
func setupSharedRedis() (func() error, error) {
	prepareContainerRuntime()
	ctx := context.Background()
	containerName := sharedContainerName(redisImage)

	var (
		addr   string
		client *goredis.Client
		index  int
	)
	err := withSharedContainerLock(containerName, func() error {
		c, err := redis.Run(
			ctx, redisImage,
			testcontainers.WithCmdArgs("--databases", strconv.Itoa(redisSharedDatabases)),
			testcontainers.WithReuseByName(containerName),
		)
		if err != nil {
			return errors.Wrap(err, "failed to start the shared redis container")
		}

		host, port, err := endpoint(ctx, c, redisPort)
		if err != nil {
			return err
		}
		addr = fmt.Sprintf("%s:%d", host, port)

		client, index, err = claimRedisDatabase(ctx, addr)
		return err
	})
	if err != nil {
		return nil, err
	}

	ApplyConfigToEnv(config.Redis{
		Addr:    addr,
		DB:      index,
		Enabled: true,
	})
	reportServiceReady("redis", fmt.Sprintf("%s/%d", addr, index))

	release := func() error {
		err := client.FlushDB(ctx).Err()
		return errors.CombineErrors(errors.Wrapf(err, "failed to flush the redis database %d", index), client.Close())
	}
	return release, nil
}

// claimRedisDatabase walks the isolation slots of the shared container and
// claims the first database index that is free or whose lease holder is dead.
// A claim flushes the database first, so leftovers of a crashed former holder
// never leak into the next binary. The caller holds the container lock, which
// is what keeps two binaries from claiming the same index.
func claimRedisDatabase(ctx context.Context, addr string) (*goredis.Client, int, error) {
	for index := 1; index < redisSharedDatabases; index++ {
		client := goredis.NewClient(&goredis.Options{Addr: addr, DB: index})

		lease, err := client.Get(ctx, redisLeaseKey).Result()
		if err != nil && !errors.Is(err, goredis.Nil) {
			return nil, 0, errors.CombineErrors(
				errors.Wrapf(err, "failed to read the lease of the redis database %d", index), client.Close(),
			)
		}
		if err == nil && redisLeaseHolderAlive(lease) {
			if err := client.Close(); err != nil {
				return nil, 0, err
			}
			continue
		}

		if err := client.FlushDB(ctx).Err(); err != nil {
			return nil, 0, errors.CombineErrors(
				errors.Wrapf(err, "failed to flush the redis database %d", index), client.Close(),
			)
		}
		if err := client.Set(ctx, redisLeaseKey, formatRedisLease(), 0).Err(); err != nil {
			return nil, 0, errors.CombineErrors(
				errors.Wrapf(err, "failed to lease the redis database %d", index), client.Close(),
			)
		}
		return client, index, nil
	}
	return nil, 0, errors.Newf("all %d databases of the shared redis container are leased", redisSharedDatabases-1)
}

// formatRedisLease returns the lease value naming this process: its pid,
// which the liveness check probes, and the claim time in unix seconds, which
// is context for a human inspecting the container.
func formatRedisLease() string {
	return fmt.Sprintf("%d:%d", os.Getpid(), time.Now().Unix())
}

// redisLeaseHolderAlive reports whether the process a lease names is still
// running, which decides between skipping the index and taking it over. An
// unparseable lease counts as alive: flushing a database this setup cannot
// account for is worse than skipping one of 255 slots.
func redisLeaseHolderAlive(lease string) bool {
	pidPart, _, ok := strings.Cut(lease, ":")
	if !ok {
		return true
	}
	pid, err := strconv.Atoi(pidPart)
	if err != nil || pid <= 0 {
		return true
	}
	return processAlive(pid)
}
