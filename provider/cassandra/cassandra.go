package cassandra

import (
	"context"
	"crypto/tls"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/gocql/gocql"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cqlpolicy"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/util"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

var (
	session *gocql.Session
	mu      sync.RWMutex
)

// init registers this provider so importing the package compiles the
// capability in and hands its lifecycle to bootstrap.
func init() {
	lifecycle.Register(lifecycle.Component{
		Name:      "cassandra",
		Stage:     lifecycle.StageProvider,
		Enabled:   func() bool { return config.App.Cassandra.Enabled },
		SetLogger: func(l types.Logger) { logger.Cassandra = l },
		Start:     start,
		Stop:      stop,
	})
}

// start initializes the global Cassandra session.
// It reads Cassandra configuration from config.App.Cassandra.
// The function is thread-safe and ensures the session is initialized only once.
func start(_ context.Context) (err error) {
	cfg := config.App.Cassandra
	mu.Lock()
	defer mu.Unlock()
	if session != nil {
		return nil
	}

	if session, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to cassandra")
	}

	zap.S().Infow("successfully connected to cassandra", "hosts", cfg.Hosts, "port", cfg.Port, "keyspace", cfg.Keyspace)

	return nil
}

// New returns a new Cassandra session with given configuration.
// It's the caller's responsibility to close the session,
// caller should always call Close() when it's no longer needed.
func New(cfg config.Cassandra) (*gocql.Session, error) {
	cluster, err := newCluster(cfg)
	if err != nil {
		return nil, err
	}
	s, err := cluster.CreateSession()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cassandra session")
	}
	return s, nil
}

// newCluster translates cfg into the driver's cluster configuration, which
// New opens the session from.
func newCluster(cfg config.Cassandra) (*gocql.ClusterConfig, error) {
	cluster := gocql.NewCluster(cfg.Hosts...)
	cluster.Port = cfg.Port

	if cfg.Keyspace != "" {
		cluster.Keyspace = cfg.Keyspace
	}
	if cfg.Username != "" && cfg.Password != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: cfg.Username,
			Password: cfg.Password,
		}
	}

	// Set consistency level; empty keeps the driver's own default. The driver
	// reads the name in any letter case and reports one it does not know;
	// its ParseConsistency panics on such a name instead.
	if cfg.Consistency != "" {
		consistency, err := gocql.ParseConsistencyWrapper(string(cfg.Consistency))
		if err != nil {
			return nil, errors.Wrapf(err, "unsupported cassandra consistency: %s", cfg.Consistency)
		}
		cluster.Consistency = consistency
	}

	// Configure timeouts
	if cfg.Timeout > 0 {
		cluster.Timeout = cfg.Timeout
	}

	if cfg.ConnectTimeout > 0 {
		cluster.ConnectTimeout = cfg.ConnectTimeout
	}

	// Connection pooling
	if cfg.NumConns > 0 {
		cluster.NumConns = cfg.NumConns
	}

	// Page size for queries
	if cfg.PageSize > 0 {
		cluster.PageSize = cfg.PageSize
	}

	// Configure retry policy
	retryPolicy, err := newRetryPolicy(cfg.RetryPolicy, cfg.MaxRetryCount, cluster.Consistency)
	if err != nil {
		return nil, err
	}
	cluster.RetryPolicy = retryPolicy

	// Configure reconnection policy; max_retry_count counts the retries of a
	// connection, its first try aside.
	if cfg.ReconnectInterval > 0 {
		cluster.ReconnectionPolicy = &gocql.ConstantReconnectionPolicy{
			MaxRetries: cqlpolicy.ReconnectAttempts(cfg.MaxRetryCount),
			Interval:   cfg.ReconnectInterval,
		}
	}

	// Configure TLS if enabled
	if cfg.TLSEnabled {
		var tlsConfig *tls.Config
		if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		cluster.SslOpts = &gocql.SslOptions{Config: tlsConfig}
	}

	return cluster, nil
}

// newRetryPolicy returns the retry policy named by policy for a session at
// level: simple and exponential_backoff retrying maxRetryCount times, an
// empty policy being the simple one, and downgrading_consistency once below
// level (see cqlpolicy.DowngradingConsistency). The configuration admits no
// other name; one set in code is an error.
func newRetryPolicy(policy config.RetryPolicy, maxRetryCount int, level gocql.Consistency) (gocql.RetryPolicy, error) {
	switch policy {
	case "", config.RetryPolicySimple:
		return &gocql.SimpleRetryPolicy{NumRetries: maxRetryCount}, nil
	case config.RetryPolicyExponentialBackoff:
		return &gocql.ExponentialBackoffRetryPolicy{NumRetries: maxRetryCount}, nil
	case config.RetryPolicyDowngradingConsistency:
		return cqlpolicy.DowngradingConsistency(level), nil
	default:
		return nil, errors.Newf("unsupported cassandra retry policy: %s", policy)
	}
}

// Client returns the initialized Cassandra session, the client handle of
// this provider.
func Client() (*gocql.Session, error) {
	mu.RLock()
	defer mu.RUnlock()
	if session == nil {
		return nil, errors.New("cassandra session not initialized")
	}
	return session, nil
}

// stop closes the global Cassandra session.
func stop(_ context.Context) error {
	mu.Lock()
	defer mu.Unlock()
	if session != nil {
		session.Close()
		zap.S().Infow("successfully close cassandra session")
		session = nil
	}
	return nil
}
