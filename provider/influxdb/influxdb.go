package influxdb

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/provider"
	"github.com/hydroan/gst/util"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/domain"
	"go.uber.org/zap"
)

var (
	client   influxdb2.Client
	writeAPI api.WriteAPIBlocking
	queryAPI api.QueryAPI
	mu       sync.RWMutex
)

// init registers this provider so importing the package compiles the
// capability in and hands its lifecycle to bootstrap.
func init() {
	provider.Register(provider.Provider{Name: "influxdb", Init: Init, Close: Close})
}

// Init initializes the global InfluxDB client.
// It reads InfluxDB configuration from config.App.Influxdb.
// If InfluxDB is not enabled, it returns nil.
// The function is thread-safe and ensures the client is initialized only once.
func Init() (err error) {
	cfg := config.App.Influxdb
	if !cfg.Enabled {
		return nil
	}

	mu.Lock()
	defer mu.Unlock()
	if client != nil {
		return nil
	}

	if client, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to create influxdb client")
	}

	// Verify connection by checking server health
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	health, err := client.Health(ctx)
	if err != nil {
		client.Close()
		client = nil
		return errors.Wrap(err, "failed to check influxdb health")
	}

	if health.Status != domain.HealthCheckStatusPass {
		client.Close()
		client = nil
		return errors.Newf("influxdb health check failed: %s", health.Status)
	}

	// Get write and query APIs
	writeAPI = client.WriteAPIBlocking(cfg.Org, cfg.Bucket)
	queryAPI = client.QueryAPI(cfg.Org)

	zap.S().Infow("successfully connected to influxdb",
		"host", cfg.Host,
		"port", cfg.Port,
		"org", cfg.Org,
		"bucket", cfg.Bucket)

	return nil
}

// New returns a new InfluxDB client instance with given configuration.
// It's the caller's responsibility to close the client,
// caller should always call client.Close() when it's no longer needed.
func New(cfg config.Influxdb) (influxdb2.Client, error) {
	opts := influxdb2.DefaultOptions()
	if cfg.BatchSize > 0 {
		opts.SetBatchSize(cfg.BatchSize)
	}
	if cfg.FlushInterval > 0 {
		opts.SetFlushInterval(uint(cfg.FlushInterval))
	}
	if cfg.RetryInterval > 0 {
		opts.SetRetryInterval(uint(cfg.RetryInterval))
	}
	if cfg.MaxRetries > 0 {
		opts.SetMaxRetries(cfg.MaxRetries)
	}
	if cfg.RetryBufferLimit > 0 {
		opts.SetRetryBufferLimit(cfg.RetryBufferLimit)
	}
	if cfg.MaxRetryInterval > 0 {
		opts.SetMaxRetryInterval(uint(cfg.MaxRetryInterval.Milliseconds())) //nolint:gosec
	}
	if cfg.MaxRetryTime > 0 {
		opts.SetMaxRetryTime(uint(cfg.MaxRetryTime.Milliseconds())) //nolint:gosec
	}
	if cfg.ExponentialBase > 0 {
		opts.SetExponentialBase(cfg.ExponentialBase)
	}
	if cfg.Precision > 0 {
		opts.SetPrecision(cfg.Precision)
	}
	if cfg.UseGZip {
		opts.SetUseGZip(true)
	}
	if cfg.TLSEnabled {
		var tlsConf *tls.Config
		var err error
		if tlsConf, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return nil, errors.Wrap(err, "failed to build tls config")
		}
		if tlsConf != nil {
			opts.SetTLSConfig(tlsConf)
		}
	}
	if cfg.DefaultTags != nil {
		for k, v := range cfg.DefaultTags {
			opts.AddDefaultTag(k, v)
		}
	}
	if len(cfg.AppName) > 0 {
		opts.SetApplicationName(cfg.AppName)
	}

	return influxdb2.NewClientWithOptions(fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port), cfg.Token, opts), nil
}

// WritePoint writes a single point to InfluxDB
func WritePoint(ctx context.Context, measurement string, tags map[string]string, fields map[string]any, ts ...time.Time) error {
	mu.RLock()
	defer mu.RUnlock()

	if client == nil {
		return errors.New("influxdb client not initialized")
	}

	// Create a point
	var t time.Time
	if len(ts) > 0 {
		t = ts[0]
	} else {
		t = time.Now()
	}

	p := influxdb2.NewPoint(measurement, tags, fields, t)

	// Write the point
	return writeAPI.WritePoint(ctx, p)
}

// Query executes a Flux query against InfluxDB
func Query(ctx context.Context, query string) (*api.QueryTableResult, error) {
	mu.RLock()
	defer mu.RUnlock()

	if client == nil {
		return nil, errors.New("influxdb client not initialized")
	}

	// Execute query
	return queryAPI.Query(ctx, query)
}

// Client returns the initialized InfluxDB client.
func Client() (influxdb2.Client, error) {
	mu.RLock()
	defer mu.RUnlock()
	if client == nil {
		return nil, errors.New("influxdb client not initialized")
	}
	return client, nil
}

// WriteAPI returns the write API of the initialized InfluxDB client.
func WriteAPI() (api.WriteAPIBlocking, error) {
	mu.RLock()
	defer mu.RUnlock()
	if writeAPI == nil {
		return nil, errors.New("influxdb client not initialized")
	}
	return writeAPI, nil
}

// QueryAPI returns the query API of the initialized InfluxDB client.
func QueryAPI() (api.QueryAPI, error) {
	mu.RLock()
	defer mu.RUnlock()
	if queryAPI == nil {
		return nil, errors.New("influxdb client not initialized")
	}
	return queryAPI, nil
}

// Close gracefully shuts down the InfluxDB client
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if client != nil {
		client.Close()
		client = nil
		writeAPI = nil
		queryAPI = nil
		zap.S().Infow("successfully closed influxdb client")
	}

	return nil
}

// Health checks the current health of the InfluxDB server
func Health() (*domain.HealthCheck, error) {
	mu.RLock()
	defer mu.RUnlock()

	if client == nil {
		return nil, errors.New("influxdb client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return client.Health(ctx)
}
