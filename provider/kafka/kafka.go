package kafka

import (
	"sync"

	"github.com/IBM/sarama"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

var (
	mu          sync.RWMutex
	initialized bool
	client      sarama.Client
)

func Init() (err error) {
	cfg := config.App.Kafka
	if !cfg.Enable {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	if client, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to create kafka client")
	}

	// Check connection
	brokers := client.Brokers()
	if len(brokers) == 0 {
		client.Close()
		client = nil
		return errors.New("failed to connect to kafka: no brokers available")
	}

	zap.S().Infow("successfully connected to kafka", "brokers", cfg.Brokers)

	initialized = true
	return nil
}

// New returns a new Kafka client instance with given configuration.
// It's the caller's responsibility to close the client,
// caller should always call Close() when it's no longer needed.
func New(cfg config.Kafka) (sarama.Client, error) {
	saramaConfig := sarama.NewConfig()
	if err := configureKafka(saramaConfig, cfg); err != nil {
		return nil, err
	}

	return sarama.NewClient(cfg.Brokers, saramaConfig)
}

// configureKafka sets up the Sarama configuration based on our app config
func configureKafka(config *sarama.Config, cfg config.Kafka) error {
	// Version configuration
	if cfg.Version != "" {
		version, err := sarama.ParseKafkaVersion(cfg.Version)
		if err != nil {
			return errors.Wrap(err, "invalid kafka version")
		}
		config.Version = version
	}

	// Client ID
	if cfg.ClientID != "" {
		config.ClientID = cfg.ClientID
	}

	// Authentication
	if cfg.SASL.Enable {
		config.Net.SASL.Enable = true
		config.Net.SASL.Mechanism = sarama.SASLMechanism(cfg.SASL.Mechanism)
		config.Net.SASL.User = cfg.SASL.Username
		config.Net.SASL.Password = cfg.SASL.Password

		// SCRAM authentication requires an external package and specific implementation
		// If using SCRAM, you'll need to implement this separately with github.com/xdg-go/scram
		if cfg.SASL.Mechanism == sarama.SASLTypeSCRAMSHA256 ||
			cfg.SASL.Mechanism == sarama.SASLTypeSCRAMSHA512 {
			return errors.New("SCRAM authentication requires additional setup. Please implement with github.com/xdg-go/scram")
		}
	}

	// TLS configuration
	if cfg.EnableTLS {
		config.Net.TLS.Enable = true
		tlsConfig, err := util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify)
		if err != nil {
			return errors.Wrap(err, "failed to build TLS config")
		}
		config.Net.TLS.Config = tlsConfig
	}

	// Producer configurations
	config.Producer.RequiredAcks = sarama.RequiredAcks(cfg.Producer.RequiredAcks) //nolint:gosec
	config.Producer.Retry.Max = cfg.Producer.Retries
	if cfg.Producer.Compression != "" {
		switch cfg.Producer.Compression {
		case "none":
			config.Producer.Compression = sarama.CompressionNone
		case "gzip":
			config.Producer.Compression = sarama.CompressionGZIP
		case "snappy":
			config.Producer.Compression = sarama.CompressionSnappy
		case "lz4":
			config.Producer.Compression = sarama.CompressionLZ4
		case "zstd":
			config.Producer.Compression = sarama.CompressionZSTD
		default:
			return errors.New("invalid compression type")
		}
	}

	if cfg.Producer.MaxMessageBytes > 0 {
		config.Producer.MaxMessageBytes = cfg.Producer.MaxMessageBytes
	}

	// Consumer configurations
	if cfg.Consumer.Group != "" {
		// Set consumer group rebalance strategy
		if cfg.Consumer.RebalanceStrategy != "" {
			switch cfg.Consumer.RebalanceStrategy {
			case "range":
				config.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRange()
			case "roundrobin":
				config.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
			case "sticky":
				config.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategySticky()
			default:
				return errors.New("invalid rebalance strategy")
			}
		}
	}

	// Initial offset configuration
	if cfg.Consumer.Offset != "" {
		switch cfg.Consumer.Offset {
		case "newest":
			config.Consumer.Offsets.Initial = sarama.OffsetNewest
		case "oldest":
			config.Consumer.Offsets.Initial = sarama.OffsetOldest
		default:
			return errors.New("invalid initial offset")
		}
	}

	// Timeout configurations
	if cfg.Timeout.Dial > 0 {
		config.Net.DialTimeout = cfg.Timeout.Dial
	}
	if cfg.Timeout.Read > 0 {
		config.Net.ReadTimeout = cfg.Timeout.Read
	}
	if cfg.Timeout.Write > 0 {
		config.Net.WriteTimeout = cfg.Timeout.Write
	}

	return nil
}

// NewAsyncProducer creates a new AsyncProducer using the given client
func NewAsyncProducer(client sarama.Client) (sarama.AsyncProducer, error) {
	return sarama.NewAsyncProducerFromClient(client)
}

// NewSyncProducer creates a new SyncProducer using the given client
func NewSyncProducer(client sarama.Client) (sarama.SyncProducer, error) {
	return sarama.NewSyncProducerFromClient(client)
}

// NewConsumerGroup creates a new ConsumerGroup with the given client
func NewConsumerGroup(client sarama.Client, groupID string) (sarama.ConsumerGroup, error) {
	return sarama.NewConsumerGroupFromClient(groupID, client)
}

func Close() {
	mu.Lock()
	defer mu.Unlock()
	if client != nil {
		if err := client.Close(); err != nil {
			zap.S().Errorw("failed to close kafka client", "error", err)
		} else {
			zap.S().Infow("successfully closed kafka client")
		}
		client = nil
	}
}
