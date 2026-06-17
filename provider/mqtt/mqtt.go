package mqtt

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

var (
	client      mqtt.Client
	mu          sync.RWMutex
	initialized bool
	clientID    string
)

func Init() (err error) {
	cfg := config.App.Mqtt
	if !cfg.Enable {
		return nil
	}

	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	if client, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to create mqtt client")
	}
	if err := connect(client); err != nil {
		client = nil
		return errors.Wrap(err, "failed to connect to mqtt broker")
	}
	zap.S().Infow(
		"successfully connect to mqtt broker",
		"addr", cfg.Addr,
		"client_id", clientID,
		"keepalive", config.App.Keepalive.String(),
		"connection_timeout", cfg.ConnectTimeout.String(),
		"clean_session", cfg.CleanSession,
		"auto_reconnect", cfg.AutoReconnect,
	)
	go monitorConnection()
	initialized = true
	return nil
}

func New(cfg config.Mqtt) (mqtt.Client, error) {
	opts, err := buildOptions(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build mqtt options")
	}
	return mqtt.NewClient(opts), nil
}

func buildOptions(cfg config.Mqtt) (*mqtt.ClientOptions, error) {
	clientID = fmt.Sprintf(
		"%s-%d",
		defaultIfEmpty(cfg.ClientPrefix, "mqtt-client"),
		rand.New(rand.NewSource(time.Now().UnixNano())).Int(), //nolint:gosec
	)

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Addr).
		SetAutoReconnect(cfg.AutoReconnect).
		SetClientID(clientID).
		SetProtocolVersion(5).
		SetKeepAlive(cfg.Keepalive).
		SetConnectTimeout(cfg.ConnectTimeout).
		SetCleanSession(cfg.CleanSession)
	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}
	if cfg.UseTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec
		}
		if len(cfg.CertFile) != 0 && len(cfg.KeyFile) != 0 {
			cert, err := loadCertificate(cfg.CertFile, cfg.KeyFile)
			if err != nil {
				return nil, errors.Wrap(err, "failed to load mqtt certificate")
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		opts.SetTLSConfig(tlsConfig)
	}
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		logger.Mqtt.Errorw("mqtt connection lost", "error", err, "client_id", clientID)
	})
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		logger.Mqtt.Infow("mqtt client connected", "client_id", clientID)
	})

	return opts, nil
}

func connect(client mqtt.Client) error {
	token := client.Connect()
	if !token.WaitTimeout(config.App.Mqtt.ConnectTimeout) {
		return errors.New("connect timeout")
	}
	if err := token.Error(); err != nil {
		return err
	}
	return nil
}

// loadCertificate loads TLS certificate
func loadCertificate(certFile, keyFile string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return tls.Certificate{}, errors.Wrap(err, "failed to load certificate")
	}
	return cert, nil
}

// defaultIfEmpty returns default value if str is empty
func defaultIfEmpty(str, defaultStr string) string {
	if str == "" {
		return defaultStr
	}
	return str
}

func monitorConnection() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !client.IsConnected() {
			logger.Mqtt.Warn("mqtt client disconnected, attempting to reconnect...")
			if err := connect(client); err != nil {
				logger.Mqtt.Errorw("reconnect failed", "error", err)
				continue
			}
			logger.Mqtt.Info("successfully reconnected")
		}
	}
}

// Client returns the MQTT client instance
func Client() (mqtt.Client, error) {
	mu.RLock()
	defer mu.RUnlock()

	if !initialized {
		return nil, errors.New("mqtt client not initialized")
	}
	if client == nil {
		return nil, errors.New("mqtt client is nil")
	}

	return client, nil
}

// Health checks if the MQTT client is connected
func Health() error {
	c, err := Client()
	if err != nil {
		return err
	}

	if !c.IsConnected() {
		return errors.New("mqtt client is not connected")
	}

	return nil
}

// Close closes the MQTT client connection
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if client != nil {
		client.Disconnect(250) // 等待 250ms 完成断开
		client = nil
		initialized = false
	}
	return nil
}

func Publish(topic string, payload any, opts ...PublishOption) error {
	c, err := Client()
	if err != nil {
		return err
	}
	opt := DefaultPublishOption
	if len(opts) > 0 {
		opt = opts[0]
	}

	var data []byte

	switch v := payload.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		var err error
		if data, err = json.Marshal(v); err != nil {
			return errors.Wrap(err, "failed to marshal payload")
		}
	}

	token := c.Publish(topic, opt.QoS, opt.Retain, data)
	if !token.WaitTimeout(opt.Timeout) {
		return errors.New("publish timeout")
	}
	if err := token.Error(); err != nil {
		logger.Mqtt.Errorw(
			"publish failed",
			"error", err,
			"topic", topic,
			"addr", config.App.Mqtt.Addr,
		)
		return err
	}
	logger.Mqtt.Debugw(
		"publish success",
		"topic", topic,
		"payload", string(data),
		"qos", opt.QoS,
	)
	return nil
}

type MessageHandler func(topic string, payload []byte) error

func Subscribe(topic string, handler MessageHandler, opts ...SubscribeOption) error {
	c, err := Client()
	if err != nil {
		return err
	}
	opt := DefaultSubscribeOption
	if len(opts) > 0 {
		opt = opts[0]
	}
	wrapper := func(client mqtt.Client, msg mqtt.Message) {
		logger.Mqtt.Debugw(
			"received message",
			"topic", msg.Topic(),
			"payload", string(msg.Payload()),
		)
		if handler != nil {
			if err := handler(msg.Topic(), msg.Payload()); err != nil {
				logger.Mqtt.Errorw(
					"handle message failed",
					"error", err,
					"topic", msg.Topic(),
				)
			}
		}
	}

	token := c.Subscribe(topic, opt.QoS, wrapper)
	if !token.WaitTimeout(opt.Timeout) {
		return errors.New("subscribe timeout")
	}
	if err := token.Error(); err != nil {
		logger.Mqtt.Errorw(
			"subscribe failed",
			"error", err,
			"topic", topic,
			"addr", config.App.Mqtt.Addr,
		)
		return err
	}

	logger.Mqtt.Infow(
		"subscribe success",
		"topic", topic,
		"qos", opt.QoS,
	)
	return nil
}

func Unsubscribe(topics ...string) error {
	c, err := Client()
	if err != nil {
		return err
	}

	token := c.Unsubscribe(topics...)
	if !token.WaitTimeout(5 * time.Second) {
		return errors.New("unsubscribe timeout")
	}
	if err := token.Error(); err != nil {
		logger.Mqtt.Errorw(
			"unsubscribe failed",
			"error", err,
			"topics", topics,
		)
		return err
	}

	logger.Mqtt.Infow("unsubscribe success", "topics", topics)
	return nil
}
