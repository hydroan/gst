package cassandra

import (
	"crypto/tls"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/gocql/gocql"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

var (
	initialized bool
	session     *gocql.Session
	mu          sync.RWMutex
)

// Init initializes the global Cassandra session.
// It reads Cassandra configuration from config.App.Cassandra.
// If Cassandra is not enabled, it returns nil.
// The function is thread-safe and ensures the session is initialized only once.
func Init() (err error) {
	cfg := config.App.Cassandra
	if !cfg.Enabled {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	if session, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to cassandra")
	}

	zap.S().Infow("successfully connected to cassandra", "hosts", cfg.Hosts, "port", cfg.Port, "keyspace", cfg.Keyspace)

	initialized = true
	return nil
}

// New returns a new Cassandra session with given configuration.
// It's the caller's responsibility to close the session,
// caller should always call Close() when it's no longer needed.
func New(cfg config.Cassandra) (*gocql.Session, error) {
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

	// Set consistency level
	if cfg.Consistency != "" {
		consistency, err := parseConsistency(cfg.Consistency)
		if err != nil {
			return nil, errors.Wrap(err, "invalid consistency level")
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
	retryPolicy, err := getRetryPolicy(cfg.RetryPolicy, cfg.MaxRetryCount)
	if err != nil {
		return nil, errors.Wrap(err, "invalid retry policy")
	}
	cluster.RetryPolicy = retryPolicy

	// Configure reconnection policy
	if cfg.ReconnectInterval > 0 {
		cluster.ReconnectionPolicy = &gocql.ConstantReconnectionPolicy{
			MaxRetries: cfg.MaxRetryCount,
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

	// Create session
	s, err := cluster.CreateSession()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cassandra session")
	}

	return s, nil
}

// Helper function to parse consistency level
func parseConsistency(consistency string) (gocql.Consistency, error) {
	switch consistency {
	case "ANY":
		return gocql.Any, nil
	case "ONE":
		return gocql.One, nil
	case "TWO":
		return gocql.Two, nil
	case "THREE":
		return gocql.Three, nil
	case "QUORUM":
		return gocql.Quorum, nil
	case "ALL":
		return gocql.All, nil
	case "LOCAL_QUORUM":
		return gocql.LocalQuorum, nil
	case "EACH_QUORUM":
		return gocql.EachQuorum, nil
	case "LOCAL_ONE":
		return gocql.LocalOne, nil
	default:
		return gocql.Quorum, errors.New("unknown consistency level: " + consistency)
	}
}

// Helper function to get the appropriate retry policy
func getRetryPolicy(policyName string, maxRetryCount int) (gocql.RetryPolicy, error) {
	switch policyName {
	case "default":
		return &gocql.SimpleRetryPolicy{NumRetries: maxRetryCount}, nil
	case "exponential":
		return &gocql.ExponentialBackoffRetryPolicy{NumRetries: maxRetryCount}, nil
	case "fallthrough":
		return &gocql.DowngradingConsistencyRetryPolicy{}, nil
	default:
		return &gocql.SimpleRetryPolicy{NumRetries: maxRetryCount}, nil
	}
}

// Session returns the global Cassandra session.
// It returns nil if the session is not initialized.
func Session() *gocql.Session {
	mu.RLock()
	defer mu.RUnlock()
	return session
}

// Close closes the global Cassandra session.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if session != nil {
		session.Close()
		zap.S().Infow("successfully close cassandra session")
		session = nil
	}
	initialized = false
}
