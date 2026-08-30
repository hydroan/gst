// Package redis holds the process-wide Redis client, the typed cache backend
// built on it, and the helpers that share its keyspace.
//
// The client is go-redis v9, which speaks to Redis 7 and newer; a deployment
// on Redis 6 or older needs the v8 client instead.
package redis

import (
	"context"
	"crypto/tls"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/util"
	jsoniter "github.com/json-iterator/go"
	redisotel "github.com/redis/go-redis/extra/redisotel/v9"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// json is the encoder every value in this package is stored and read with.
// The sonic library is about twice as fast as the standard library, but is
// not portable to every platform the framework targets.
var json = jsoniter.ConfigCompatibleWithStandardLibrary

var (
	// cli is the one client handle this package holds. Standalone and cluster
	// connections both satisfy it, so every operation reads the same variable
	// under the same lock: a handle that is absent — never initialized, or
	// closed during shutdown — is absent for all of them at once.
	cli goredis.UniversalClient
	mu  sync.RWMutex

	ErrKeyNotExists    = errors.New("key no longer exists, may be expired")
	ErrRedisIsDisabled = errors.New("redis is disabled")
)

// Init connects the process-wide client and verifies it with a ping, then
// installs tracing and metrics on it. A deployment that leaves Redis disabled
// keeps no handle, which every operation reports through Client. A second
// call on a connected process does nothing.
func Init() (err error) {
	cfg := config.App.Redis
	if !cfg.Enabled {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if cli != nil {
		return nil
	}

	if cfg.ClusterMode {
		var cluster *goredis.ClusterClient
		if cluster, err = NewCluster(cfg); err != nil {
			return errors.Wrap(err, "failed to connect to redis")
		}
		cli = cluster
		zap.S().Infow("successfully connect to redis", "addrs", cfg.Addrs, "cluster_mode", cfg.ClusterMode)
	} else {
		var client *goredis.Client
		if client, err = New(cfg); err != nil {
			return errors.Wrap(err, "failed to connect to redis")
		}
		cli = client
		zap.S().Infow("successfully connect to redis", "addr", cfg.Addr, "db", cfg.DB, "cluster_mode", cfg.ClusterMode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = cli.Ping(ctx).Err(); err != nil {
		cli.Close()
		cli = nil
		return errors.Wrap(err, "failed to ping redis")
	}
	if err = errors.Join(redisotel.InstrumentTracing(cli), redisotel.InstrumentMetrics(cli)); err != nil {
		cli.Close()
		cli = nil
		return err
	}

	return nil
}

// New builds a standalone client from cfg without installing it as the
// process-wide handle. Init builds its handle with it, and a caller needing a
// connection of its own — one pointed at another database, say — can too.
func New(cfg config.Redis) (*goredis.Client, error) {
	opts := &goredis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	if cfg.PoolSize > 0 {
		opts.PoolSize = cfg.PoolSize
	}
	if cfg.DialTimeout > 0 {
		opts.DialTimeout = cfg.DialTimeout
	}
	if cfg.ReadTimeout > 0 {
		opts.ReadTimeout = cfg.ReadTimeout
	}
	if cfg.WriteTimeout > 0 {
		opts.WriteTimeout = cfg.WriteTimeout
	}
	if cfg.MinIdleConns > 0 {
		opts.MinIdleConns = cfg.MinIdleConns
	}
	if cfg.MaxRetries > 0 {
		opts.MaxRetries = cfg.MaxRetries
	}
	if cfg.MinRetryBackoff > 0 {
		opts.MinRetryBackoff = cfg.MinRetryBackoff
	}
	if cfg.MaxRetryBackoff > 0 {
		opts.MaxRetryBackoff = cfg.MaxRetryBackoff
	}
	if cfg.TLSEnabled {
		var tlsConfig *tls.Config
		var err error
		if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		opts.TLSConfig = tlsConfig
	}

	return goredis.NewClient(opts), nil
}

// NewCluster is New for a cluster deployment, reading cfg.Addrs instead of
// the single address.
func NewCluster(cfg config.Redis) (*goredis.ClusterClient, error) {
	opts := &goredis.ClusterOptions{
		Addrs:    cfg.Addrs,
		Password: cfg.Password,
	}
	if cfg.PoolSize > 0 {
		opts.PoolSize = cfg.PoolSize
	}
	if cfg.DialTimeout > 0 {
		opts.DialTimeout = cfg.DialTimeout
	}
	if cfg.ReadTimeout > 0 {
		opts.ReadTimeout = cfg.ReadTimeout
	}
	if cfg.WriteTimeout > 0 {
		opts.WriteTimeout = cfg.WriteTimeout
	}
	if cfg.MinIdleConns > 0 {
		opts.MinIdleConns = cfg.MinIdleConns
	}
	if cfg.MaxRetries > 0 {
		opts.MaxRetries = cfg.MaxRetries
	}
	if cfg.MinRetryBackoff > 0 {
		opts.MinRetryBackoff = cfg.MinRetryBackoff
	}
	if cfg.MaxRetryBackoff > 0 {
		opts.MaxRetryBackoff = cfg.MaxRetryBackoff
	}
	if cfg.TLSEnabled {
		var tlsConfig *tls.Config
		var err error
		if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		opts.TLSConfig = tlsConfig
	}

	return goredis.NewClusterClient(opts), nil
}

// Client returns the initialized Redis client handle, standalone or
// cluster depending on configuration.
//
// It reports ErrRedisIsDisabled whenever no handle is held, which covers a
// deployment that never enabled Redis, one whose Init failed, and a process
// past Close. Every operation in this package goes through it, so an absent
// handle is an error at the call site instead of a zero value the caller
// cannot tell from a real answer.
func Client() (goredis.UniversalClient, error) {
	mu.RLock()
	defer mu.RUnlock()
	if cli == nil {
		return nil, ErrRedisIsDisabled
	}
	return cli, nil
}

// Close releases the connection and drops the handle, so operations that
// arrive afterwards report ErrRedisIsDisabled instead of reaching a closed
// connection. A process that never connected reports no error.
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if cli == nil {
		return nil
	}
	err := cli.Close()
	cli = nil
	if err != nil {
		return errors.Wrap(err, "failed to close redis client")
	}
	zap.S().Infow("successfully close redis client", "cluster_mode", config.App.Redis.ClusterMode)
	return nil
}
