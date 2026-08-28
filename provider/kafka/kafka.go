package kafka

import (
	"context"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/provider"
	"github.com/hydroan/gst/util"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	"go.uber.org/zap"
)

// pingTimeout bounds the connectivity check performed during Init.
const pingTimeout = 10 * time.Second

var (
	mu     sync.RWMutex
	client *kgo.Client
)

// init registers this provider so importing the package compiles the
// capability in and hands its lifecycle to bootstrap.
func init() {
	provider.Register(provider.Provider{Name: "kafka", Logger: &logger.Kafka, Init: initProvider, Close: closeProvider})
}

// initProvider initializes the global Kafka client backed by franz-go.
// It reads Kafka configuration from config.App.Kafka.
// If Kafka is not enabled, it returns nil.
// The function is thread-safe and ensures the client is initialized only once.
func initProvider() (err error) {
	cfg := config.App.Kafka
	if !cfg.Enabled {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if client != nil {
		return nil
	}

	var c *kgo.Client
	if c, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to create kafka client")
	}

	// Check connection.
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err = c.Ping(ctx); err != nil {
		c.Close()
		return errors.Wrap(err, "failed to connect to kafka")
	}

	zap.S().Infow("successfully connected to kafka", "brokers", cfg.Brokers, "client_id", cfg.ClientID)

	client = c
	return nil
}

// New returns a new Kafka client with the given configuration.
// Framework-level options (brokers, client id, SASL, TLS, logging) are derived
// from cfg; callers append franz-go options for their own use case, e.g.
// kgo.ConsumerGroup and kgo.ConsumeTopics for consumers. Caller options are
// applied after framework options and take precedence on conflict.
// It's the caller's responsibility to close the client,
// caller should always call Close() when it's no longer needed.
func New(cfg config.Kafka, opts ...kgo.Opt) (*kgo.Client, error) {
	base, err := buildOpts(cfg)
	if err != nil {
		return nil, err
	}
	return kgo.NewClient(append(base, opts...)...)
}

// buildOpts translates framework configuration into franz-go client options:
// brokers, logging, client id, SASL and TLS. It is the single owner of that
// translation — every framework component building a kafka client of its own
// goes through New on top of it, so a connection concern such as SASL or TLS
// is configured once and honored everywhere.
func buildOpts(cfg config.Kafka) ([]kgo.Opt, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		Logger(&logger.Kafka),
	}
	if cfg.ClientID != "" {
		opts = append(opts, kgo.ClientID(cfg.ClientID))
	}

	if cfg.SASLEnabled {
		switch cfg.SASLMechanism {
		case config.KafkaSASLMechanismPlain:
			opts = append(opts, kgo.SASL(plain.Auth{User: cfg.SASLUsername, Pass: cfg.SASLPassword}.AsMechanism()))
		case config.KafkaSASLMechanismScramSHA256:
			opts = append(opts, kgo.SASL(scram.Auth{User: cfg.SASLUsername, Pass: cfg.SASLPassword}.AsSha256Mechanism()))
		case config.KafkaSASLMechanismScramSHA512:
			opts = append(opts, kgo.SASL(scram.Auth{User: cfg.SASLUsername, Pass: cfg.SASLPassword}.AsSha512Mechanism()))
		default:
			return nil, errors.Newf("unsupported kafka sasl mechanism: %s", cfg.SASLMechanism)
		}
	}

	if cfg.TLSEnabled {
		tlsConfig, err := util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		opts = append(opts, kgo.DialTLSConfig(tlsConfig))
	}

	return opts, nil
}

// Client returns the default Kafka client instance.
// The default client carries no consumer subscription and is intended for
// producing and administrative use; consumers should create their own client
// via New with consumer options.
func Client() (*kgo.Client, error) {
	mu.RLock()
	defer mu.RUnlock()
	if client == nil {
		return nil, errors.New("kafka client not initialized")
	}
	return client, nil
}

// Admin returns an admin client backed by the default Kafka client.
// It shares the default client's connections; closing it is not required.
func Admin() (*kadm.Client, error) {
	c, err := Client()
	if err != nil {
		return nil, err
	}
	return kadm.NewClient(c), nil
}

// closeProvider closes the default Kafka client,
// allowing a subsequent Init to establish a fresh client.
func closeProvider() error {
	mu.Lock()
	defer mu.Unlock()
	if client != nil {
		client.Close()
		zap.S().Infow("successfully closed kafka client")
		client = nil
	}
	return nil
}
