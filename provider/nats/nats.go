package nats

import (
	"crypto/tls"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/provider"
	"github.com/hydroan/gst/util"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

var (
	mu   sync.RWMutex
	conn *nats.Conn
)

// init registers this provider so importing the package compiles the
// capability in and hands its lifecycle to bootstrap.
func init() {
	provider.Register(provider.Provider{Name: "nats", Init: Init, Close: Close})
}

// Init initializes the global NATS client.
// It reads NATS configuration from config.App.NatsConfig.
// If NATS is not enabled, it returns nil.
// The function is thread-safe and ensures the client is initialized only once.
func Init() (err error) {
	cfg := config.App.Nats
	if !cfg.Enabled {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if conn != nil {
		return nil
	}

	if conn, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to nats")
	}

	// Check connection status
	if !conn.IsConnected() {
		conn.Close()
		conn = nil
		return errors.New("failed to connect to nats: connection status check failed")
	}
	zap.S().Infow("successfully connect to nats", "url", cfg.Addrs, "client_name", cfg.ClientName)

	return nil
}

// New returns a new NATS client instance with given configuration.
// It's the caller's responsibility to close the client,
// caller should always call Close() when it's no longer needed.
func New(cfg config.Nats) (*nats.Conn, error) {
	opts, err := connectOptions(cfg)
	if err != nil {
		return nil, err
	}
	return nats.Connect(strings.Join(cfg.Addrs, ","), opts...)
}

// connectOptions translates the framework configuration into NATS client
// options.
func connectOptions(cfg config.Nats) ([]nats.Option, error) {
	var err error
	opts := []nats.Option{}

	if len(cfg.ClientName) > 0 {
		opts = append(opts, nats.Name(cfg.ClientName))
	}
	if len(cfg.Username) > 0 && len(cfg.Password) > 0 {
		opts = append(opts, nats.UserInfo(cfg.Username, cfg.Password))
	}
	if len(cfg.Token) > 0 {
		opts = append(opts, nats.Token(cfg.Token))
	}
	if len(cfg.CredentialsFile) > 0 {
		opts = append(opts, nats.UserCredentials(cfg.CredentialsFile))
	}
	if len(cfg.NKeyFile) > 0 {
		opt, e := nats.NkeyOptionFromSeed(cfg.NKeyFile)
		if e != nil {
			return nil, errors.Wrap(e, "failed to load nkey from seed file")
		}
		opts = append(opts, opt)
	}

	// Handed over verbatim: -1 reconnects forever (the framework default),
	// 0 disables reconnecting, a positive value bounds the attempts. Gating
	// this on > 0 would silently swap -1 and 0 for the NATS library default
	// of 60 attempts, which permanently closes the connection once spent.
	opts = append(opts, nats.MaxReconnects(cfg.MaxReconnects))
	if cfg.ReconnectWait > 0 {
		opts = append(opts, nats.ReconnectWait(cfg.ReconnectWait))
	}
	if cfg.ReconnectJitter > 0 {
		opts = append(opts, nats.ReconnectJitter(cfg.ReconnectJitter, cfg.ReconnectJitterTLS))
	}

	if cfg.ConnectTimeout > 0 {
		opts = append(opts, nats.Timeout(cfg.ConnectTimeout))
	}
	if cfg.PingInterval > 0 {
		opts = append(opts, nats.PingInterval(cfg.PingInterval))
	}
	if cfg.MaxPingsOutstanding > 0 {
		opts = append(opts, nats.MaxPingsOutstanding(cfg.MaxPingsOutstanding))
	}

	if cfg.TLSEnabled {
		var tlsConfig *tls.Config
		if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		opts = append(opts, nats.Secure(tlsConfig))
	}

	// Add connection handlers
	opts = append(opts, nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
		if err != nil {
			logger.Nats.Warnw("disconnected from nats", "error", err)
		}
	}))
	opts = append(opts, nats.ReconnectHandler(func(nc *nats.Conn) {
		logger.Nats.Infow("reconnected to nats", "url", nc.ConnectedUrl())
	}))
	opts = append(opts, nats.ClosedHandler(func(nc *nats.Conn) {
		logger.Nats.Warnw("nats connection closed")
	}))
	opts = append(opts, nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
		logger.Nats.Errorw("nats error", "error", err, "subject", sub.Subject)
	}))

	return opts, nil
}

// Client returns the initialized NATS connection, the client handle of
// this provider.
func Client() (*nats.Conn, error) {
	mu.RLock()
	defer mu.RUnlock()
	if conn == nil {
		return nil, errors.New("nats connection not initialized")
	}
	return conn, nil
}

func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if conn != nil {
		url := conn.ConnectedUrl()
		conn.Close()
		zap.S().Infow("successfully close nats client", "url", url, "client_name", config.App.Nats.ClientName)
		conn = nil
	}
	return nil
}
