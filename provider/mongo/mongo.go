package mongo

import (
	"context"
	"crypto/tls"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/provider"
	"github.com/hydroan/gst/util"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"go.uber.org/zap"
)

var (
	client *mongo.Client
	mu     sync.RWMutex
)

// init registers this provider so importing the package compiles the
// capability in and hands its lifecycle to bootstrap.
func init() {
	provider.Register(provider.Provider{Name: "mongo", Logger: &logger.Mongo, Init: initProvider, Close: closeProvider})
}

// initProvider initializes the global MongoDB client.
// It reads MongoDB configuration from config.App.MongoConfig.
// If MongoDB is not enabled, it returns nil.
// The function is thread-safe and ensures the client is initialized only once.
func initProvider() (err error) {
	cfg := config.App.Mongo
	if !cfg.Enabled {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if client != nil {
		return nil
	}

	if client, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to mongodb")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = client.Ping(ctx, readpref.Primary()); err != nil {
		client = nil
		return errors.Wrap(err, "failed to connect to mongodb")
	}
	zap.S().Infow("successfully connect to mongodb", "host", cfg.Host, "port", cfg.Port, "database", cfg.Database)

	return nil
}

// New returns a new MongoDB client instance with given configuration.
// It's the caller's responsibility to close the client,
// caller should always call Close() when it's no longer needed.
func New(cfg config.Mongo) (*mongo.Client, error) {
	var err error
	uri := buildURI(cfg)
	opts := options.Client().ApplyURI(uri)
	if cfg.MaxPoolSize > 0 {
		opts.SetMaxPoolSize(cfg.MaxPoolSize)
	}
	if cfg.MinPoolSize > 0 {
		opts.SetMinPoolSize(cfg.MinPoolSize)
	}
	if cfg.ConnectTimeout > 0 {
		opts.SetConnectTimeout(cfg.ConnectTimeout)
	}
	if cfg.ServerSelectionTimeout > 0 {
		opts.SetServerSelectionTimeout(cfg.ServerSelectionTimeout)
	}
	if cfg.MaxConnIdleTime > 0 {
		opts.SetMaxConnIdleTime(cfg.MaxConnIdleTime)
	}
	if cfg.MaxConnecting > 0 {
		opts.SetMaxConnecting(cfg.MaxConnecting)
	}
	if len(cfg.ReadConcern) > 0 {
		var rc *readconcern.ReadConcern
		switch cfg.ReadConcern {
		case config.ReadConcernLocal:
			rc = readconcern.Local()
		case config.ReadConcernMajority:
			rc = readconcern.Majority()
		case config.ReadConcernAvailable:
			rc = readconcern.Available()
		case config.ReadConcernLinearizable:
			rc = readconcern.Linearizable()
		case config.ReadConcernSnapshot:
			rc = readconcern.Snapshot()
		}
		if rc != nil {
			opts.SetReadConcern(rc)
		}
	}
	if len(cfg.WriteConcern) > 0 {
		var wc *writeconcern.WriteConcern
		switch cfg.WriteConcern {
		case config.WriteConcernMajority:
			wc = writeconcern.Majority()
		case config.WriteConcernJournaled:
			wc = writeconcern.Journaled()
		case config.WriteConcernW0:
			wc = writeconcern.Unacknowledged()
		case config.WriteConcernW1:
			wc = writeconcern.W1()
		case config.WriteConcernW2:
			wc = &writeconcern.WriteConcern{W: 2}
		case config.WriteConcernW3:
			wc = &writeconcern.WriteConcern{W: 3}
		case config.WriteConcernW4:
			wc = &writeconcern.WriteConcern{W: 4}
		case config.WriteConcernW5:
			wc = &writeconcern.WriteConcern{W: 5}
		case config.WriteConcernW6:
			wc = &writeconcern.WriteConcern{W: 6}
		case config.WriteConcernW7:
			wc = &writeconcern.WriteConcern{W: 7}
		case config.WriteConcernW8:
			wc = &writeconcern.WriteConcern{W: 8}
		case config.WriteConcernW9:
			wc = &writeconcern.WriteConcern{W: 9}
		default:
			if cfg.WriteConcern[0] >= '0' && cfg.WriteConcern[0] <= '9' {
				var w int
				if w, err = strconv.Atoi(string(cfg.WriteConcern)); err == nil && w > 1 {
					wc = &writeconcern.WriteConcern{W: w}
				}
			} else {
				wc = writeconcern.Custom(string(cfg.WriteConcern))
			}
		}
		if wc != nil {
			opts.SetWriteConcern(wc)
		}
	}
	if cfg.TLSEnabled {
		var tlsConfig *tls.Config
		if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		opts.SetTLSConfig(tlsConfig)
	}
	return mongo.Connect(opts)
}

func buildURI(cfg config.Mongo) string {
	uri := fmt.Sprintf(
		"mongodb://%s:%s@%s:%d/%s?authSource=%s",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port,
		cfg.Database, cfg.AuthSource,
	)
	if len(cfg.Username) == 0 && len(cfg.Password) == 0 {
		uri = fmt.Sprintf("mongodb://%s:%d/%s", cfg.Host, cfg.Port, cfg.Database)
	}
	return uri
}

// Client returns the MongoDB client instance
func Client() (*mongo.Client, error) {
	mu.RLock()
	defer mu.RUnlock()
	if client == nil {
		return nil, errors.New("mongo client not initialized")
	}
	if client == nil {
		return nil, errors.New("mongo client is nil")
	}
	return client, nil
}

// Database returns a handle to the specified database
func Database(name string) (*mongo.Database, error) {
	c, err := Client()
	if err != nil {
		return nil, err
	}
	return c.Database(name), nil
}

// Collection returns a handle to the specified collection
func Collection(dbName, collName string) (*mongo.Collection, error) {
	db, err := Database(dbName)
	if err != nil {
		return nil, err
	}
	return db.Collection(collName), nil
}

// closeProvider closes the MongoDB client connection
func closeProvider() error {
	mu.Lock()
	defer mu.Unlock()

	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.Disconnect(ctx); err != nil {
			return errors.Wrap(err, "failed to disconnect MongoDB client")
		}
		client = nil
	}
	return nil
}

// Health checks if the MongoDB connection is healthy
func Health() error {
	c, err := Client()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First-hand exit of a stack-less mongo-driver error; see the error-stack
	// contract in the database package doc. WithStack passes nil through.
	return errors.WithStack(c.Ping(ctx, readpref.Primary()))
}
