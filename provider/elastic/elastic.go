package elastic

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

const defaultSearchSize = 10000

// timestampFile persists the SearchTimestamp cursor across restarts.
var timestampFile = filepath.Join(os.TempDir(), "gst_elastic_timestamp")

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

// start initializes the global elasticsearch client.
// It reads elasticsearch configuration from config.App.Elasticsearch.
func start(_ context.Context) (err error) {
	cfg := config.App.Elasticsearch
	if client, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to create elasticsearch client")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Ping(client.Ping.WithContext(ctx)); err != nil {
		client = nil
		return errors.Wrap(err, "failed to ping elasticsearch")
	}

	zap.S().Infow("successfully connect to elasticsearch", "hosts", cfg.Addrs)
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

// SearchTimestamp searches index for documents newer than the timestamp persisted in
// timestampFile and returns the raw response body.
// index accepts a wildcard, eg "sample-*", to query multiple indices at once.
func SearchTimestamp(index string, size ...int) ([]byte, error) {
	_size := defaultSearchSize
	if len(size) > 0 {
		if size[0] > 0 {
			_size = size[0]
		}
	}

	// query := `
	// {
	//   "size":3000,
	//   "sort": [{"@timestamp":{"order":"asc"}}],
	//   "query": {
	//     "bool": {
	//       "must": [
	// 			{"range":{"@timestamp":{"gte":"2024-02-27T01:00:00Z","lte":"2024-02-27T23:00:00Z"}}},
	// 			{"term":{"event.action.keyword":"Logon"}}
	//     ]
	//     }
	//   }
	// }`

	var (
		err                error
		timestampEnd       time.Time
		timestampStart     time.Time
		timestampStartData []byte
	)

	now := time.Now().UTC()
	timestampEnd = time.Date(2099, now.Month(), now.Day(), 23, 59, 59, 0, time.UTC)
	if timestampStartData, err = os.ReadFile(timestampFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			timestampStart = time.Date(now.Year(), now.Month(), now.Day()-1, now.Hour(), now.Minute(), now.Second(), 0, time.UTC) // one day earlier
			fmt.Println("------------------- touch file and write time: ", timestampStart.Format(time.RFC3339))
			if err = os.WriteFile(timestampFile, []byte(timestampStart.Format(time.RFC3339)), 0o600); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		if timestampStart, err = time.ParseInLocation(time.RFC3339, string(bytes.TrimSpace(timestampStartData)), time.UTC); err != nil {
			return nil, err
		}
	}

	queryFormat := `{
  "size":%d,
  "sort": [{"@timestamp":{"order":"asc"}}],
  "query": {
	"bool": {
	  "must": [
			{"range":{"@timestamp":{"gte":"%s","lte":"%s"}}},
			{"wildcard":{"event.action.keyword": "*"}}
	]
	}
  }
}`

	query := fmt.Sprintf(queryFormat, _size, timestampStart.Format(time.RFC3339), timestampEnd.Format(time.RFC3339))
	// fmt.Println(query)

	res, err := client.Search(
		client.Search.WithContext(context.Background()),
		// client.Search.WithIndex("sample-*"), // use a wildcard to query multiple indices
		client.Search.WithIndex(index), // index may be a wildcard covering multiple indices
		client.Search.WithBody(strings.NewReader(query)),
		client.Search.WithTrackTotalHits(true),
		client.Search.WithPretty(),
	)
	if err != nil {
		panic(err)
	}
	if res.IsError() {
		fmt.Println("------------------- error")
		fmt.Println(res.String())
		return nil, errors.New(res.Status())
	}

	defer res.Body.Close()

	return io.ReadAll(res.Body)
}

type Pagination struct {
	Page int // page number
	Size int // page size
}
