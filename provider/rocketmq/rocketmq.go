package rocketmq

import (
	"sync"
	"time"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/admin"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
	"github.com/apache/rocketmq-client-go/v2/rlog"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

var (
	initialized     bool
	defaultProducer rocketmq.Producer
	defaultConsumer rocketmq.PushConsumer
	defaultAdmin    admin.Admin
	mu              sync.RWMutex
)

// Init initializes the global RocketMQ producer.
// It reads RocketMQ configuration from config.App.RocketMQ.
// If RocketMQ is not enabled, it returns nil.
// The function is thread-safe and ensures the producer is initialized only once.
func Init() (err error) {
	cfg := config.App.RocketMQ
	if !cfg.Enable {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	rlog.SetLogger(&customLogger{})
	if defaultProducer, err = NewProducer(cfg); err != nil {
		return errors.Wrap(err, "failed to create rocketmq producer")
	}
	if defaultConsumer, err = NewPushConsumer(cfg, cfg.GroupName); err != nil {
		_ = defaultProducer.Shutdown()
		defaultProducer = nil
		return errors.Wrap(err, "failed to create rocketmq consumer")
	}
	if defaultAdmin, err = NewAdmin(cfg); err != nil {
		_ = defaultProducer.Shutdown()
		_ = defaultConsumer.Shutdown()
		defaultProducer = nil
		defaultConsumer = nil
		return errors.Wrap(err, "failed to create rocketmq admin")
	}

	zap.S().Infow("successfully connect to rocketmq", "nameserver", cfg.NameServerAddrs, "group", cfg.GroupName)

	initialized = true
	return nil
}

// NewProducer returns a new RocketMQ producer with given configuration.
// It's the caller's responsibility to start and shutdown the producer.
func NewProducer(cfg config.RocketMQ) (rocketmq.Producer, error) {
	var opts []producer.Option

	opts = append(
		opts, producer.WithNameServer(cfg.NameServerAddrs),
		producer.WithVIPChannel(cfg.VipChannelEnabled),
	)

	if cfg.GroupName != "" {
		opts = append(opts, producer.WithGroupName(cfg.GroupName))
	}
	if cfg.InstanceName != "" {
		opts = append(opts, producer.WithInstanceName(cfg.InstanceName))
	}
	if cfg.Namespace != "" {
		opts = append(opts, producer.WithNamespace(cfg.Namespace))
	}
	if cfg.NumRetries > 0 {
		opts = append(opts, producer.WithRetry(cfg.NumRetries))
	}

	if cfg.SendMsgTimeout > 0 {
		opts = append(opts, producer.WithSendMsgTimeout(cfg.SendMsgTimeout))
	}

	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, producer.WithCredentials(primitive.Credentials{
			AccessKey: cfg.AccessKey,
			SecretKey: cfg.SecretKey,
		}))
	}

	if cfg.TraceEnabled {
		opts = append(opts, producer.WithTrace(&primitive.TraceConfig{}))
	}

	// if cfg.EnableTLS {
	// 	var tlsConfig *tls.Config
	// 	var err error
	// 	if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
	// 		return nil, errors.Wrap(err, "failed to build TLS config")
	// 	}
	// 	opts = append(opts, producer.WithTLS(tlsConfig))
	// }

	p, err := producer.NewDefaultProducer(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create rocketmq producer")
	}

	return p, nil
}

// NewPushConsumer returns a new RocketMQ push consumer with given configuration.
// It's the caller's responsibility to start and shutdown the consumer.
func NewPushConsumer(cfg config.RocketMQ, consumerGroup string) (rocketmq.PushConsumer, error) {
	if consumerGroup == "" {
		consumerGroup = cfg.GroupName
	}

	opts := []consumer.Option{
		consumer.WithNameServer(cfg.NameServerAddrs),
		consumer.WithGroupName(consumerGroup),
		consumer.WithAutoCommit(cfg.AutoCommit),
	}
	if cfg.InstanceName != "" {
		opts = append(opts, consumer.WithInstance(cfg.InstanceName))
	}
	if cfg.Namespace != "" {
		opts = append(opts, consumer.WithNamespace(cfg.Namespace))
	}

	switch cfg.ConsumeFromWhere {
	case config.ConsumeFromWhereFirstOffset:
		opts = append(opts, consumer.WithConsumeFromWhere(consumer.ConsumeFromFirstOffset))
	case config.ConsumeFromWhereTimestamp:
		if cfg.ConsumeTimestamp != "" {
			opts = append(opts, consumer.WithConsumeTimestamp(cfg.ConsumeTimestamp))
		} else {
			opts = append(opts, consumer.WithConsumeTimestamp(time.Now().Format("20060102150405")))
		}
	default:
		opts = append(opts, consumer.WithConsumeFromWhere(consumer.ConsumeFromLastOffset))
	}
	if cfg.ConsumeOrderly {
		opts = append(opts, consumer.WithConsumerOrder(true))
	}
	if cfg.MaxReconsumeTime > 0 {
		opts = append(opts, consumer.WithMaxReconsumeTimes(cfg.MaxReconsumeTime))
	}
	if cfg.ConsumeConcurrentlyMaxSpan > 0 {
		opts = append(opts, consumer.WithConsumeConcurrentlyMaxSpan(cfg.ConsumeConcurrentlyMaxSpan))
	}
	if cfg.ConsumeMessageBatchMaxSize > 0 {
		opts = append(opts, consumer.WithConsumeMessageBatchMaxSize(cfg.ConsumeMessageBatchMaxSize))
	}
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, consumer.WithCredentials(primitive.Credentials{
			AccessKey: cfg.AccessKey,
			SecretKey: cfg.SecretKey,
		}))
	}
	if cfg.TraceEnabled {
		opts = append(opts, consumer.WithTrace(&primitive.TraceConfig{}))
	}

	// if cfg.EnableTLS {
	// 	var tlsConfig *tls.Config
	// 	var err error
	// 	if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
	// 		return nil, errors.Wrap(err, "failed to build TLS config")
	// 	}
	// 	opts = append(opts, consumer.WithTLS(tlsConfig))
	// }

	c, err := consumer.NewPushConsumer(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create rocketmq consumer")
	}

	return c, nil
}

// NewAdmin returns a new RocketMQ admin with given configuration.
// It's the caller's responsibility to close the admin when it's no longer needed.
func NewAdmin(cfg config.RocketMQ) (admin.Admin, error) {
	var opts []admin.AdminOption

	opts = append(opts, admin.WithResolver(primitive.NewPassthroughResolver(cfg.NameServerAddrs)))

	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, admin.WithCredentials(primitive.Credentials{
			AccessKey: cfg.AccessKey,
			SecretKey: cfg.SecretKey,
		}))
	}

	// if cfg.EnableTLS {
	// 	var tlsConfig *tls.Config
	// 	var err error
	// 	if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
	// 		return nil, errors.Wrap(err, "failed to build TLS config")
	// 	}
	// 	opts = append(opts, admin.WithTLS(tlsConfig))
	// }

	c, err := admin.NewAdmin(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create rocketmq admin")
	}

	return c, nil
}

// Producer returns the default RocketMQ producer instance
func Producer() (rocketmq.Producer, error) {
	mu.RLock()
	defer mu.RUnlock()
	if !initialized {
		return nil, errors.New("rocketmq producer not initialized, call Init() first")
	}
	if defaultProducer == nil {
		return nil, errors.New("rocketmq producer is nil")
	}
	return defaultProducer, nil
}

// Consumer returns the default RocketMQ consumer instance
func Consumer() (rocketmq.PushConsumer, error) {
	mu.RLock()
	defer mu.RUnlock()
	if !initialized {
		return nil, errors.New("rocketmq consumer not initialized, call Init() first")
	}
	if defaultConsumer == nil {
		return nil, errors.New("rocketmq consumer is nil")
	}
	return defaultConsumer, nil
}

// Admin returns the default RocketMQ admin instance
func Admin() (admin.Admin, error) {
	mu.RLock()
	defer mu.RUnlock()
	if !initialized {
		return nil, errors.New("rocketmq admin not initialized, call Init() first")
	}
	if defaultAdmin == nil {
		return nil, errors.New("rocketmq admin is nil")
	}
	return defaultAdmin, nil
}

// Close closes the RocketMQ producer connection
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if defaultProducer != nil {
		if err := defaultProducer.Shutdown(); err != nil {
			zap.S().Errorw("failed to shutdown rocketmq producer", "error", err)
		} else {
			zap.S().Infow("successfully shutdown rocketmq producer")
		}
		defaultProducer = nil
	}
	if defaultConsumer != nil {
		if err := defaultConsumer.Shutdown(); err != nil {
			zap.S().Errorw("failed to shutdown rocketmq consumer", "error", err)
		} else {
			zap.S().Infow("successfully shutdown rocketmq consumer")
		}
		defaultConsumer = nil
	}
	if defaultAdmin != nil {
		if err := defaultAdmin.Close(); err != nil {
			zap.S().Errorw("failed to close rocketmq admin", "error", err)
		} else {
			zap.S().Infow("successfully close rocketmq admin")
		}
		defaultAdmin = nil
	}
	initialized = false
}

type customLogger struct{}

func (l *customLogger) Debug(msg string, fields map[string]any) { logger.RocketMQ.Debug(msg, fields) }

func (l *customLogger) Info(msg string, fields map[string]any) { logger.RocketMQ.Info(msg, fields) }

func (l *customLogger) Warning(msg string, fields map[string]any) { logger.RocketMQ.Warn(msg, fields) }

func (l *customLogger) Error(msg string, fields map[string]any) { logger.RocketMQ.Error(msg, fields) }

func (l *customLogger) Fatal(msg string, fields map[string]any) { logger.RocketMQ.Fatal(msg, fields) }
func (l *customLogger) Level(string)                            {}
func (l *customLogger) OutputPath(string) error                 { return nil }
