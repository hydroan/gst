package scylla

import (
	"context"
	"crypto/tls"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/gocql/gocql"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cqlpolicy"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/logfield"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/util"
	"github.com/hydroan/gst/logger"
	"github.com/scylladb/gocqlx/v3"
	"go.uber.org/zap"
)

var (
	session gocqlx.Session
	mu      sync.RWMutex
)

// init registers this provider so importing the package compiles the
// capability in and hands its lifecycle to bootstrap.
func init() {
	lifecycle.Register(lifecycle.Component{
		Name:      "scylla",
		Stage:     lifecycle.StageProvider,
		Enabled:   func() bool { return config.App.Scylla.Enabled },
		SetLogger: func(l types.Logger) { logger.Scylla = l },
		Start:     start,
		Stop:      stop,
	})
}

// start initializes the global ScyllaDB session.
// It reads ScyllaDB configuration from config.App.Scylla.
// The function is thread-safe and ensures the session is initialized only once.
func start(_ context.Context) (err error) {
	cfg := config.App.Scylla
	mu.Lock()
	defer mu.Unlock()
	if session.Session != nil {
		return nil
	}

	if session, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to ScyllaDB")
	}

	// Test the connection by executing a simple query
	if err = session.ExecStmt("SELECT release_version FROM system.local"); err != nil {
		session.Close()
		return errors.Wrap(err, "failed to connect to ScyllaDB")
	}

	zap.S().Infow("successfully connected to ScyllaDB", "hosts", cfg.Hosts, "keyspace", cfg.Keyspace)

	return nil
}

// New returns a new ScyllaDB session with given configuration.
// It's the caller's responsibility to close the session,
// caller should always call Close() when it's no longer needed.
func New(cfg config.Scylla) (gocqlx.Session, error) {
	cluster, err := newCluster(cfg)
	if err != nil {
		return gocqlx.Session{}, err
	}
	return gocqlx.WrapSession(gocql.NewSession(*cluster))
}

// newCluster translates cfg into the driver's cluster configuration, which
// New opens the session from.
func newCluster(cfg config.Scylla) (*gocql.ClusterConfig, error) {
	cluster := gocql.NewCluster(cfg.Hosts...)

	if len(cfg.Username) > 0 && len(cfg.Password) > 0 {
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: cfg.Username,
			Password: cfg.Password,
		}
	}
	if len(cfg.Keyspace) > 0 {
		cluster.Keyspace = cfg.Keyspace
	}
	// An empty consistency keeps the driver's own default. The driver reads
	// the name in any letter case and reports one it does not know; its
	// ParseConsistency panics on such a name instead.
	if cfg.Consistency != "" {
		level, err := gocql.ParseConsistencyWrapper(string(cfg.Consistency))
		if err != nil {
			return nil, errors.Wrapf(err, "unsupported scylla consistency: %s", cfg.Consistency)
		}
		cluster.Consistency = level
	}
	if cfg.NumConns > 0 {
		cluster.NumConns = cfg.NumConns
	}
	if cfg.ConnectTimeout > 0 {
		cluster.ConnectTimeout = cfg.ConnectTimeout
	}
	if cfg.Timeout > 0 {
		cluster.Timeout = cfg.Timeout
	}
	if cfg.PageSize > 0 {
		cluster.PageSize = cfg.PageSize
	}

	// Configure retry policy; an empty policy keeps the driver's own default.
	switch cfg.RetryPolicy {
	case "":
	case config.RetryPolicySimple:
		cluster.RetryPolicy = &gocql.SimpleRetryPolicy{NumRetries: cfg.RetryNumRetries}
	case config.RetryPolicyDowngradingConsistency:
		cluster.RetryPolicy = cqlpolicy.DowngradingConsistency(cluster.Consistency)
	case config.RetryPolicyExponentialBackoff:
		cluster.RetryPolicy = &gocql.ExponentialBackoffRetryPolicy{
			NumRetries: cfg.RetryNumRetries,
			Min:        cfg.RetryMinInterval,
			Max:        cfg.RetryMaxInterval,
		}
	default:
		return nil, errors.Newf("unsupported scylla retry policy: %s", cfg.RetryPolicy)
	}

	// Configure reconnect policy; an empty policy keeps the driver's own
	// default. reconnect_max_retries counts the retries of a connection, its
	// first try aside.
	switch cfg.ReconnectPolicy {
	case "":
	case config.ReconnectPolicyConstant:
		cluster.ReconnectionPolicy = &gocql.ConstantReconnectionPolicy{
			Interval:   cfg.ReconnectConstantInterval,
			MaxRetries: cqlpolicy.ReconnectAttempts(cfg.ReconnectMaxRetries),
		}
	case config.ReconnectPolicyExponential:
		cluster.ReconnectionPolicy = &gocql.ExponentialReconnectionPolicy{
			MaxRetries:      cqlpolicy.ReconnectAttempts(cfg.ReconnectMaxRetries),
			InitialInterval: cfg.ReconnectInitialInterval,
			MaxInterval:     cfg.ReconnectMaxInterval,
		}
	default:
		return nil, errors.Newf("unsupported scylla reconnect policy: %s", cfg.ReconnectPolicy)
	}

	if cfg.TracingEnabled {
		cluster.QueryObserver = &queryObserver{}
		cluster.BatchObserver = &batchObserver{}
	}

	if cfg.TLSEnabled {
		var tlsConfig *tls.Config
		var err error
		if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		cluster.SslOpts = &gocql.SslOptions{Config: tlsConfig}
	}

	return cluster, nil
}

// queryObserver implements gocql.QueryObserver to trace queries.
type queryObserver struct{}

func (o *queryObserver) ObserveQuery(ctx context.Context, query gocql.ObservedQuery) {
	logger.Scylla.Infow(
		"ScyllaDB query",
		"statement", query.Statement,
		"values", query.Values,
		"keyspace", query.Keyspace,
		logfield.Duration(query.End.Sub(query.Start)),
		"error", query.Err,
	)
}

// batchObserver implements gocql.BatchObserver to trace batches.
type batchObserver struct{}

func (o *batchObserver) ObserveBatch(ctx context.Context, batch gocql.ObservedBatch) {
	statements := make([]string, 0, len(batch.Statements))
	statements = append(statements, batch.Statements...)

	logger.Scylla.Infow(
		"ScyllaDB batch",
		"statements", statements,
		"values", batch.Values,
		"keyspace", batch.Keyspace,
		logfield.Duration(batch.End.Sub(batch.Start)),
		"error", batch.Err,
	)
}

// Client returns the initialized ScyllaDB session, the client handle of
// this provider.
func Client() (gocqlx.Session, error) {
	mu.RLock()
	defer mu.RUnlock()
	if session.Session == nil {
		return gocqlx.Session{}, errors.New("scylla session not initialized")
	}
	return session, nil
}

// stop closes the ScyllaDB session
func stop(_ context.Context) error {
	mu.Lock()
	defer mu.Unlock()

	if session.Session != nil {
		session.Close()
		zap.S().Infow("successfully closed ScyllaDB session")
		session = gocqlx.Session{}
	}
	return nil
}
