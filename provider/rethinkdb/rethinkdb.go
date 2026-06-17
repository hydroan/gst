package rethinkdb

import (
	"crypto/tls"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

var (
	initialized bool
	session     *r.Session
	mu          sync.RWMutex
)

// Init initializes the global RethinkDB session.
// It reads RethinkDB configuration from config.App.RethinkDB.
// If RethinkDB is not enabled, it returns nil.
// The function is thread-safe and ensures the session is initialized only once.
func Init() (err error) {
	cfg := config.App.RethinkDB
	if !cfg.Enable {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	if session, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to rethinkdb")
	}

	if _, err = r.Expr("ping").Run(session); err != nil {
		session.Close()
		session = nil
		return errors.Wrap(err, "failed to connect to rethinkdb")
	}
	zap.S().Infow("successfully connect to rethinkdb", "hosts", cfg.Hosts, "database", cfg.Database)

	initialized = true
	return nil
}

// New returns a new RethinkDB session with given configuration.
// It's the caller's responsibility to close the session,
// caller should always call Close() when it's no longer needed.
func New(cfg config.RethinkDB) (*r.Session, error) {
	opts := r.ConnectOpts{
		Addresses:     cfg.Hosts,
		Database:      cfg.Database,
		Username:      cfg.Username,
		Password:      cfg.Password,
		DiscoverHosts: cfg.DiscoveryHost,
	}
	if cfg.MaxIdle > 0 {
		// opts.MaxIdle = cfg.MaxIdle
		opts.InitialCap = cfg.MaxIdle
	}
	if cfg.MaxOpen > 0 {
		opts.MaxOpen = cfg.MaxOpen
	}
	if cfg.NumRetries > 0 {
		opts.NumRetries = cfg.NumRetries
	}

	if cfg.ConnectTimeout > 0 {
		opts.Timeout = cfg.ConnectTimeout
	}
	if cfg.ReadTimeout > 0 {
		opts.ReadTimeout = cfg.ReadTimeout
	}
	if cfg.WriteTimeout > 0 {
		opts.WriteTimeout = cfg.WriteTimeout
	}
	if cfg.KeepAliveTime > 0 {
		opts.KeepAlivePeriod = cfg.KeepAliveTime
	}

	if cfg.EnableTLS {
		var tlsConfig *tls.Config
		var err error
		if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		opts.TLSConfig = tlsConfig
	}

	_session, err := r.Connect(opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to rethinkdb")
	}
	return _session, nil
}

// Session returns the RethinkDB session instance
func Session() (*r.Session, error) {
	mu.RLock()
	defer mu.RUnlock()
	if !initialized {
		return nil, errors.New("rethinkdb session not initialized, call Init() first")
	}
	if session == nil {
		return nil, errors.New("rethinkdb session is nil")
	}
	return session, nil
}

// Close closes the RethinkDB session
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if session != nil {
		if err := session.Close(); err != nil {
			zap.S().Errorw("failed to close rethinkdb session", "error", err)
		} else {
			zap.S().Infow("successfully closed rethinkdb session")
		}
		session = nil
		initialized = false
	}
}

// Health checks if the RethinkDB connection is healthy
func Health() error {
	s, err := Session()
	if err != nil {
		return err
	}

	_, err = r.Expr("ping").Run(s)
	return err
}
