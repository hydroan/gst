package elastic

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// pingTimeout bounds the connectivity check start performs.
const pingTimeout = 5 * time.Second

type (
	document struct{}
	index    struct{}
)

var (
	client *elasticsearch.Client

	Document = new(document)
	Index    = new(index)
)

// init registers this provider so importing the package compiles the
// capability in and hands its lifecycle to bootstrap.
func init() {
	lifecycle.Register(lifecycle.Component{
		Name:      "elastic",
		Stage:     lifecycle.StageProvider,
		Enabled:   func() bool { return config.App.Elasticsearch.Enabled },
		SetLogger: func(l types.Logger) { logger.Elastic = l },
		Start:     start,
	})
}

// start creates the framework's client from config.App.Elasticsearch and
// confirms the cluster answers a ping before handing it out: a cluster that
// refuses the request, wrong credentials included, answers the ping with an
// error status and is reported here rather than on the first use.
func start(_ context.Context) error {
	cfg := config.App.Elasticsearch
	c, err := New(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to create elasticsearch client")
	}
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	res, err := c.Ping(c.Ping.WithContext(ctx))
	if err != nil {
		return errors.Wrap(err, "failed to ping elasticsearch")
	}
	defer res.Body.Close()
	if res.IsError() {
		return errors.Newf("failed to ping elasticsearch: %s", res.Status())
	}

	zap.S().Infow("successfully connected to elasticsearch", "hosts", cfg.Addrs)
	client = c
	return nil
}

// New returns a new Elasticsearch client with given configuration.
// It's the caller's responsibility to ensure proper usage of the client.
func New(cfg config.Elasticsearch) (*elasticsearch.Client, error) {
	// Create the Elasticsearch configuration
	esCfg := elasticsearch.Config{Addresses: cfg.Addrs}

	// Set basic authentication if provided
	if cfg.Username != "" && cfg.Password != "" {
		esCfg.Username = cfg.Username
		esCfg.Password = cfg.Password
	}

	// Set CloudID if provided
	if cfg.CloudID != "" {
		esCfg.CloudID = cfg.CloudID
	}

	// Set API Key if provided
	if cfg.APIKey != "" {
		esCfg.APIKey = cfg.APIKey
	}

	// Configure retries
	if !cfg.DisableRetries {
		esCfg.RetryOnStatus = cfg.RetryOnStatus
		esCfg.MaxRetries = cfg.MaxRetries

		if cfg.RetryBackoff {
			esCfg.RetryBackoff = func(attempt int) time.Duration {
				// Calculate exponential backoff with min and max bounds
				retryDelay := min(cfg.RetryBackoffMin*time.Duration(1<<uint(attempt)), cfg.RetryBackoffMax)
				return retryDelay
			}
		}
	} else {
		esCfg.DisableRetry = true
	}

	// Configure compression
	esCfg.CompressRequestBody = cfg.Compress

	// Configure discovery interval
	if cfg.DiscoveryInterval > 0 {
		esCfg.DiscoverNodesInterval = cfg.DiscoveryInterval
	}

	// Configure metrics
	esCfg.EnableMetrics = cfg.MetricsEnabled

	// Configure debug logger
	if cfg.DebugLoggerEnabled {
		esCfg.Logger = &elasticLogger{logger.Elastic}
	}

	// Configure transport
	transport := &http.Transport{
		MaxIdleConnsPerHost:   cfg.ConnectionPoolSize,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		DialContext: (&net.Dialer{
			Timeout:   cfg.DialTimeout,
			KeepAlive: cfg.KeepAliveInterval,
		}).DialContext,
	}

	// Configure TLS if enabled
	if cfg.TLSEnabled {
		var tlsConfig *tls.Config
		tlsConfig, err := util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		transport.TLSClientConfig = tlsConfig
	}
	esCfg.Transport = transport

	// Create the client
	cli, err := elasticsearch.NewClient(esCfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create elasticsearch client")
	}
	return cli, nil
}

// _check will check the client and return an error if it's nil or invalid.
func _check() error {
	if client == nil {
		return errors.New("elasticsearch client not initialized")
	}
	return nil
}

// Client returns the initialized Elasticsearch client.
func Client() (*elasticsearch.Client, error) {
	if err := _check(); err != nil {
		return nil, err
	}
	return client, nil
}
