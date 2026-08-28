package rethinkdb

import (
	"crypto/tls"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/provider"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

var (
	session *r.Session
	mu      sync.RWMutex
)

// init registers this provider so importing the package compiles the
// capability in and hands its lifecycle to bootstrap.
func init() {
	provider.Register(provider.Provider{Name: "rethinkdb", Logger: &logger.RethinkDB, Init: initProvider, Close: closeProvider})
}

// initProvider initializes the global RethinkDB session.
// It reads RethinkDB configuration from config.App.RethinkDB.
// If RethinkDB is not enabled, it returns nil.
// The function is thread-safe and ensures the session is initialized only once.
func initProvider() (err error) {
	cfg := config.App.RethinkDB
	if !cfg.Enabled {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if session != nil {
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

	if cfg.TLSEnabled {
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

// Client returns the initialized RethinkDB session, the client handle of
// this provider.
func Client() (*r.Session, error) {
	mu.RLock()
	defer mu.RUnlock()
	if session == nil {
		return nil, errors.New("rethinkdb session not initialized")
	}
	if session == nil {
		return nil, errors.New("rethinkdb session is nil")
	}
	return session, nil
}

// closeProvider closes the RethinkDB session
func closeProvider() error {
	mu.Lock()
	defer mu.Unlock()

	if session == nil {
		return nil
	}
	err := session.Close()
	session = nil
	if err != nil {
		return errors.Wrap(err, "failed to close rethinkdb session")
	}
	zap.S().Infow("successfully closed rethinkdb session")
	return nil
}

// Health checks if the RethinkDB connection is healthy
func Health() error {
	s, err := Client()
	if err != nil {
		return err
	}

	_, err = r.Expr("ping").Run(s)
	return err
}
