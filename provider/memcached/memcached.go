package memcached

import (
	"sync"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"go.uber.org/zap"
)

var (
	initialized bool
	client      *memcache.Client
	mu          sync.RWMutex
)

// Init initializes the global Memcached client.
// It reads Memcached configuration from config.App.Memcached.
// If Memcached is not enabled, it returns nil.
// The function is thread-safe and ensures the client is initialized only once.
func Init() (err error) {
	cfg := config.App.Memcached
	if !cfg.Enable {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	if client, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to memcached")
	}

	if err = client.Ping(); err != nil {
		client.Close()
		client = nil
		return errors.Wrap(err, "failed to connect to memcached")
	}
	zap.S().Infow("successfully connect to memcached", "servers", cfg.Servers)

	initialized = true
	return nil
}

// New returns a new Memcached client instance with given configuration.
// It's the caller's responsibility to close the client if needed.
func New(cfg config.Memcached) (*memcache.Client, error) {
	if len(cfg.Servers) == 0 {
		return nil, errors.New("memcached servers not configured")
	}

	mc := memcache.New(cfg.Servers...)
	if cfg.MaxIdleConns > 0 {
		mc.MaxIdleConns = cfg.MaxIdleConns
	}
	if cfg.Timeout > 0 {
		mc.Timeout = cfg.Timeout
	}
	// if cfg.MaxCacheSize > 0 {
	// 	mc.MaxCacheSize = cfg.MaxCacheSize
	// }

	return mc, nil
}

// Client returns the Memcached client instance
func Client() (*memcache.Client, error) {
	mu.RLock()
	defer mu.RUnlock()
	if !initialized {
		return nil, errors.New("memcached client not initialized, call Init() first")
	}
	if client == nil {
		return nil, errors.New("memcached client is nil")
	}
	return client, nil
}

// Set stores a key-value pair with expiration time
func Set(key string, value []byte, expiration int32) error {
	c, err := Client()
	if err != nil {
		return err
	}
	return c.Set(&memcache.Item{
		Key:        key,
		Value:      value,
		Expiration: expiration,
	})
}

// Get retrieves a value by key
func Get(key string) ([]byte, error) {
	c, err := Client()
	if err != nil {
		return nil, err
	}
	item, err := c.Get(key)
	if err != nil {
		return nil, err
	}
	return item.Value, nil
}

// Delete removes a key-value pair
func Delete(key string) error {
	c, err := Client()
	if err != nil {
		return err
	}
	return c.Delete(key)
}

// GetMulti retrieves multiple values by keys
func GetMulti(keys []string) (map[string]*memcache.Item, error) {
	c, err := Client()
	if err != nil {
		return nil, err
	}
	return c.GetMulti(keys)
}

// Increment increments a counter by delta
func Increment(key string, delta uint64) (uint64, error) {
	c, err := Client()
	if err != nil {
		return 0, err
	}
	return c.Increment(key, delta)
}

// Decrement decrements a counter by delta
func Decrement(key string, delta uint64) (uint64, error) {
	c, err := Client()
	if err != nil {
		return 0, err
	}
	return c.Decrement(key, delta)
}

// Close closes the Memcached client connection
// Note: gomemcache doesn't have a Close method, but we keep this for API consistency
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if client != nil {
		if err := client.Close(); err != nil {
			zap.S().Errorw("failed to close memcached client", "error", err)
		} else {
			zap.S().Infow("successfully close memcached client", "servers", config.App.Memcached.Servers)
		}
		client = nil
		initialized = false
	}
}

// Health checks if the Memcached connection is healthy
func Health() error {
	c, err := Client()
	if err != nil {
		return err
	}
	return c.Ping()
}
