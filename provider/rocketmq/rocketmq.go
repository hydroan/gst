package rocketmq

import (
	"context"
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
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

var (
	defaultProducer rocketmq.Producer
	defaultAdmin    admin.Admin
	mu              sync.RWMutex
)

// connectTimeout bounds the connectivity check start performs.
const connectTimeout = 10 * time.Second

// init registers this provider so importing the package compiles the
// capability in and hands its lifecycle to bootstrap, and hands the client
// library's log lines to the provider's logger: the library keeps one logger
// per process and reads it from every client goroutine, so it is set before
// any client runs.
func init() {
	rlog.SetLogger(&customLogger{l: &logger.RocketMQ})
	lifecycle.Register(lifecycle.Component{
		Name:      "rocketmq",
		Stage:     lifecycle.StageProvider,
		Enabled:   func() bool { return config.App.RocketMQ.Enabled },
		SetLogger: func(l types.Logger) { logger.RocketMQ = l },
		Start:     start,
		Stop:      stop,
	})
}

// start creates the framework's producer and admin client, confirms the name
// servers answer by listing their topics and starts the producer, so Producer
// hands out an instance that sends. It runs once; a later call finds the
// producer and returns.
func start(_ context.Context) error {
	cfg := config.App.RocketMQ
	mu.Lock()
	defer mu.Unlock()
	if defaultProducer != nil {
		return nil
	}

	p, err := NewProducer(cfg)
	if err != nil {
		return err
	}
	a, err := NewAdmin(cfg)
	if err != nil {
		_ = p.Shutdown()
		return err
	}

	// Check connection: the client library starts a producer without
	// contacting anything and would report an unreachable name server on
	// the first send.
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	if _, err = a.FetchAllTopicList(ctx); err != nil {
		_ = a.Close()
		_ = p.Shutdown()
		return errors.Wrap(err, "failed to connect to rocketmq")
	}
	if err = p.Start(); err != nil {
		_ = a.Close()
		_ = p.Shutdown()
		return errors.Wrap(err, "failed to start rocketmq producer")
	}

	zap.S().Infow("successfully connected to rocketmq", "nameserver", cfg.NameServerAddrs, "group", cfg.GroupName)

	defaultProducer, defaultAdmin = p, a
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
	case "", config.ConsumeFromWhereLastOffset:
		opts = append(opts, consumer.WithConsumeFromWhere(consumer.ConsumeFromLastOffset))
	case config.ConsumeFromWhereFirstOffset:
		opts = append(opts, consumer.WithConsumeFromWhere(consumer.ConsumeFromFirstOffset))
	case config.ConsumeFromWhereTimestamp:
		if cfg.ConsumeTimestamp != "" {
			opts = append(opts, consumer.WithConsumeTimestamp(cfg.ConsumeTimestamp))
		} else {
			opts = append(opts, consumer.WithConsumeTimestamp(time.Now().Format("20060102150405")))
		}
	default:
		return nil, errors.Newf("unsupported rocketmq consume_from_where: %s", cfg.ConsumeFromWhere)
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

	c, err := admin.NewAdmin(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create rocketmq admin")
	}

	return c, nil
}

// Producer returns the framework's producer, started by the bootstrap and
// ready to send.
func Producer() (rocketmq.Producer, error) {
	mu.RLock()
	defer mu.RUnlock()
	if defaultProducer == nil {
		return nil, errors.New("rocketmq producer not initialized")
	}
	return defaultProducer, nil
}

// Admin returns the framework's admin client.
func Admin() (admin.Admin, error) {
	mu.RLock()
	defer mu.RUnlock()
	if defaultAdmin == nil {
		return nil, errors.New("rocketmq admin not initialized")
	}
	return defaultAdmin, nil
}

// stop shuts down the framework's producer and closes its admin client.
func stop(_ context.Context) error {
	mu.Lock()
	defer mu.Unlock()

	var errs []error
	if defaultProducer != nil {
		if err := defaultProducer.Shutdown(); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to shutdown rocketmq producer"))
		} else {
			zap.S().Infow("successfully shutdown rocketmq producer")
		}
		defaultProducer = nil
	}
	if defaultAdmin != nil {
		if err := defaultAdmin.Close(); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to close rocketmq admin"))
		} else {
			zap.S().Infow("successfully close rocketmq admin")
		}
		defaultAdmin = nil
	}
	return errors.Join(errs...)
}

// customLogger adapts the provider's logger to the client library's logger
// interface. It dereferences l on every line, so the logger the bootstrap
// installs after the package has loaded receives the lines, and a process
// that never ran the logging setup, which leaves the logger nil, drops them,
// the way the kafka provider's logger does.
type customLogger struct {
	l *types.Logger
}

func (c *customLogger) Debug(msg string, fields map[string]any) {
	if l := *c.l; l != nil {
		l.Debug(msg, fields)
	}
}

func (c *customLogger) Info(msg string, fields map[string]any) {
	if l := *c.l; l != nil {
		l.Info(msg, fields)
	}
}

func (c *customLogger) Warning(msg string, fields map[string]any) {
	if l := *c.l; l != nil {
		l.Warn(msg, fields)
	}
}

func (c *customLogger) Error(msg string, fields map[string]any) {
	if l := *c.l; l != nil {
		l.Error(msg, fields)
	}
}

func (c *customLogger) Fatal(msg string, fields map[string]any) {
	if l := *c.l; l != nil {
		l.Fatal(msg, fields)
	}
}

func (*customLogger) Level(string)            {}
func (*customLogger) OutputPath(string) error { return nil }
