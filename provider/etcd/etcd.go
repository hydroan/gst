package etcd

import (
	"context"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/provider"
	"github.com/hydroan/gst/util"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

var (
	client *clientv3.Client
	mu     sync.RWMutex
)

// init registers this provider so importing the package compiles the
// capability in and hands its lifecycle to bootstrap.
func init() {
	provider.Register(provider.Provider{Name: "etcd", Logger: &logger.Etcd, Init: initProvider, Close: closeProvider})
}

// initProvider initializes the global etcd client.
// It reads etcd configuration from config.App.Etcd.
// If etcd is not enabled, it returns nil.
// The function is thread-safe and ensures the client is initialized only once.
func initProvider() (err error) {
	cfg := config.App.Etcd
	if !cfg.Enabled {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if client != nil {
		return nil
	}

	if client, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to etcd")
	}

	// Try to establish a connection to etcd and verify the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt to get cluster status to verify connection
	if _, err = client.Status(ctx, cfg.Endpoints[0]); err != nil {
		// Close the client connection to avoid resource leaks
		client.Close()
		client = nil
		return errors.Wrap(err, "failed to connect to etcd")
	}

	zap.S().Infow("successfully connected to etcd", "endpoints", cfg.Endpoints)

	return nil
}

// New returns a new etcd client instance with given configuration.
// It's the caller's responsibility to close the client,
// caller should always call Close() when it's no longer needed.
func New(cfg config.Etcd) (*clientv3.Client, error) {
	if len(cfg.Endpoints) == 0 {
		return nil, errors.New("no etcd endpoints provided")
	}
	etcdConfig := clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: cfg.DialTimeout,
		Logger:      logger.Etcd.(*pkgzap.Logger).ZapLogger(), //nolint:errcheck
	}

	// Set username and password authentication if provided
	if cfg.Username != "" && cfg.Password != "" {
		etcdConfig.Username = cfg.Username
		etcdConfig.Password = cfg.Password
	}

	// Configure auto sync
	if cfg.AutoSync {
		etcdConfig.AutoSyncInterval = cfg.AutoSyncInterval
	}

	// Configure keepalive settings
	if cfg.KeepAliveTime > 0 {
		etcdConfig.DialKeepAliveTime = cfg.KeepAliveTime
	}
	if cfg.KeepAliveTimeout > 0 {
		etcdConfig.DialKeepAliveTimeout = cfg.KeepAliveTimeout
	}

	// Configure message size limits
	if cfg.MaxCallSendMsgSize > 0 {
		etcdConfig.MaxCallSendMsgSize = cfg.MaxCallSendMsgSize
	}
	if cfg.MaxCallRecvMsgSize > 0 {
		etcdConfig.MaxCallRecvMsgSize = cfg.MaxCallRecvMsgSize
	}

	// Other options
	etcdConfig.PermitWithoutStream = cfg.PermitWithoutStream
	etcdConfig.RejectOldCluster = cfg.RejectOldCluster

	// Configure TLS if enabled
	if cfg.TLSEnabled {
		tlsConfig, err := util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		etcdConfig.TLS = tlsConfig
	}

	// Create client
	cli, err := clientv3.New(etcdConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create etcd client")
	}

	return cli, nil
}

// Client returns the initialized etcd client.
func Client() (*clientv3.Client, error) {
	mu.RLock()
	defer mu.RUnlock()
	if client == nil {
		return nil, errors.New("etcd client not initialized")
	}
	return client, nil
}

// closeProvider closes the global etcd client.
func closeProvider() error {
	mu.Lock()
	defer mu.Unlock()
	if client == nil {
		return nil
	}
	err := client.Close()
	client = nil
	if err != nil {
		return errors.Wrap(err, "failed to close etcd client")
	}
	zap.S().Infow("successfully closed etcd client")
	return nil
}
