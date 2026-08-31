package scylla

import (
	"context"
	"crypto/tls"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gocql/gocql"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/provider"
	"github.com/hydroan/gst/util"
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
	provider.Register(provider.Provider{
		Name:    "scylla",
		Enabled: func() bool { return config.App.Scylla.Enabled },
		Logger:  &logger.Scylla,
		Init:    initProvider,
		Close:   closeProvider,
	})
}

// initProvider initializes the global ScyllaDB session.
// It reads ScyllaDB configuration from config.App.Scylla.
// The function is thread-safe and ensures the session is initialized only once.
func initProvider() (err error) {
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
	// NOTE: you cannot call ParseConsistency, because it will panic
	// cluster.Consistency = gocql.ParseConsistency(string(cfg.Consistency))
	cluster.Consistency = parseConsistencyLevel(cfg.Consistency)
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

	// Configure retry policy
	switch cfg.RetryPolicy {
	case config.RetryPolicySimple:
		cluster.RetryPolicy = &gocql.SimpleRetryPolicy{NumRetries: cfg.RetryNumRetries}
	case config.RetryPolicyDowngradingConsistency:
		// Create a downgrading consistency retry policy that tries multiple consistency levels
		consistencyLevels := []gocql.Consistency{
			gocql.Quorum,
			gocql.One,
			gocql.Any,
		}

		// If a specific consistency was configured, put it first in the list
		if len(cfg.Consistency) > 0 {
			configuredConsistency := parseConsistencyLevel(cfg.Consistency)
			// Create a new slice with the configured consistency first
			newConsistencyLevels := make([]gocql.Consistency, 0, len(consistencyLevels)+1)
			newConsistencyLevels = append(newConsistencyLevels, configuredConsistency)

			// Add the other consistency levels that are different
			for _, c := range consistencyLevels {
				if c != configuredConsistency {
					newConsistencyLevels = append(newConsistencyLevels, c)
				}
			}

			consistencyLevels = newConsistencyLevels
		}

		cluster.RetryPolicy = &gocql.DowngradingConsistencyRetryPolicy{
			ConsistencyLevelsToTry: consistencyLevels,
		}
	case config.RetryPolicyExponentialBackoff:
		cluster.RetryPolicy = &gocql.ExponentialBackoffRetryPolicy{
			NumRetries: cfg.RetryNumRetries,
			Min:        cfg.RetryMinInterval,
			Max:        cfg.RetryMaxInterval,
		}
	}

	// Configure reconnect policy
	switch cfg.ReconnectPolicy {
	case config.ReconnectPolicyConstant:
		cluster.ReconnectionPolicy = &gocql.ConstantReconnectionPolicy{
			Interval:   cfg.ReconnectConstantInterval,
			MaxRetries: cfg.ReconnectMaxRetries,
		}
	case config.ReconnectPolicyExponential:
		cluster.ReconnectionPolicy = &gocql.ExponentialReconnectionPolicy{
			MaxRetries:      cfg.ReconnectMaxRetries,
			InitialInterval: cfg.ReconnectInitialInterval,
			MaxInterval:     cfg.ReconnectMaxInterval,
		}
	}

	if cfg.TracingEnabled {
		cluster.QueryObserver = &queryObserver{}
		cluster.BatchObserver = &batchObserver{}
	}

	if cfg.TLSEnabled {
		var tlsConfig *tls.Config
		var err error
		if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return gocqlx.Session{}, errors.Wrap(err, "failed to build TLS config")
		}
		cluster.SslOpts = &gocql.SslOptions{Config: tlsConfig}
	}

	return gocqlx.WrapSession(gocql.NewSession(*cluster))
}

// queryObserver implements gocql.QueryObserver to trace queries.
type queryObserver struct{}

func (o *queryObserver) ObserveQuery(ctx context.Context, query gocql.ObservedQuery) {
	logger.Scylla.Infow(
		"ScyllaDB query",
		"statement", query.Statement,
		"values", query.Values,
		"keyspace", query.Keyspace,
		util.LogDuration(query.End.Sub(query.Start)),
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
		util.LogDuration(batch.End.Sub(batch.Start)),
		"error", batch.Err,
	)
}

// parseConsistencyLevel converts a string consistency level to gocql.Consistency
func parseConsistencyLevel(level config.Consistency) gocql.Consistency {
	switch {
	case strings.EqualFold(string(level), gocql.Any.String()):
		return gocql.Any
	case strings.EqualFold(string(level), gocql.One.String()):
		return gocql.One
	case strings.EqualFold(string(level), gocql.Two.String()):
		return gocql.Two
	case strings.EqualFold(string(level), gocql.Three.String()):
		return gocql.Three
	case strings.EqualFold(string(level), gocql.Quorum.String()):
		return gocql.Quorum
	case strings.EqualFold(string(level), gocql.All.String()):
		return gocql.All
	case strings.EqualFold(string(level), gocql.LocalQuorum.String()):
		return gocql.LocalQuorum
	case strings.EqualFold(string(level), gocql.EachQuorum.String()):
		return gocql.EachQuorum
	case strings.EqualFold(string(level), gocql.LocalOne.String()):
		return gocql.LocalOne
	default:
		return gocql.Quorum
	}
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

// closeProvider closes the ScyllaDB session
func closeProvider() error {
	mu.Lock()
	defer mu.Unlock()

	if session.Session != nil {
		session.Close()
		zap.S().Infow("successfully closed ScyllaDB session")
		session = gocqlx.Session{}
	}
	return nil
}

// Health checks if the ScyllaDB connection is healthy
func Health() error {
	sess, err := Client()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Execute a simple query to check connectivity
	q := sess.ContextQuery(ctx, "SELECT release_version FROM system.local", nil)
	var version string
	if err := q.Get(&version); err != nil {
		return errors.Wrap(err, "ScyllaDB health check failed")
	}

	return nil
}
