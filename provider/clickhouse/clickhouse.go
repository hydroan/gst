// Package clickhouse provides the native ClickHouse client for analytical
// workloads: high-throughput batch ingestion and queries that bypass the
// gorm dialect. The gorm dialect and this provider share the clickhouse
// configuration section: the dialect serves ClickHouse as the primary
// database when database.type selects it, while this provider serves the
// native workload alongside whatever primary database the project runs.
package clickhouse

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/provider"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
)

// pingTimeout bounds the connectivity check performed during Init.
const pingTimeout = 5 * time.Second

var (
	mu   sync.RWMutex
	conn driver.Conn
)

// init registers this provider so importing the package compiles the
// capability in and hands its lifecycle to bootstrap.
func init() {
	provider.Register(provider.Provider{
		Name:    "clickhouse",
		Enabled: func() bool { return config.App.Clickhouse.Enabled },
		Init:    initProvider,
		Close:   closeProvider,
	})
}

// initProvider initializes the global native ClickHouse connection.
// It reads the configuration from config.App.Clickhouse.
func initProvider() (err error) {
	cfg := config.App.Clickhouse
	mu.Lock()
	defer mu.Unlock()
	if conn != nil {
		return nil
	}

	c, err := New(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to create clickhouse connection")
	}

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err = c.Ping(ctx); err != nil {
		_ = c.Close()
		return errors.Wrap(err, "failed to connect to clickhouse")
	}

	zap.S().Infow("successfully connect to clickhouse", "host", cfg.Host, "port", cfg.Port, "database", cfg.Database)

	conn = c
	return nil
}

// New returns a new native ClickHouse connection with the given
// configuration. Callers own the returned connection and must close it when
// it is no longer needed.
func New(cfg config.Clickhouse) (driver.Conn, error) {
	opts := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
	}
	if cfg.Compress {
		opts.Compression = &clickhouse.Compression{Method: clickhouse.CompressionLZ4}
	}
	if cfg.Debug {
		opts.Logger = slog.New(zapslog.NewHandler(zap.L().Core()))
	}
	if cfg.DialTimeout != "" {
		d, err := time.ParseDuration(cfg.DialTimeout)
		if err != nil {
			return nil, errors.Wrap(err, "invalid clickhouse dial_timeout")
		}
		opts.DialTimeout = d
	}
	if cfg.ReadTimeout != "" {
		d, err := time.ParseDuration(cfg.ReadTimeout)
		if err != nil {
			return nil, errors.Wrap(err, "invalid clickhouse read_timeout")
		}
		opts.ReadTimeout = d
	}
	return clickhouse.Open(opts)
}

// Client returns the initialized native ClickHouse connection. Batch
// ingestion talks to it directly: PrepareBatch, Append, Send.
func Client() (driver.Conn, error) {
	mu.RLock()
	defer mu.RUnlock()
	if conn == nil {
		return nil, errors.New("clickhouse connection not initialized")
	}
	return conn, nil
}

// closeProvider closes the global native ClickHouse connection.
func closeProvider() error {
	mu.Lock()
	defer mu.Unlock()
	if conn == nil {
		return nil
	}
	err := conn.Close()
	conn = nil
	if err != nil {
		return errors.Wrap(err, "failed to close clickhouse connection")
	}
	zap.S().Infow("successfully close clickhouse connection")
	return nil
}
