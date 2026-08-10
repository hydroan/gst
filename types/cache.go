package types

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
)

// ErrEntryNotFound is returned when a cache entry is not found.
var ErrEntryNotFound = errors.New("cache entry not found")

// ErrTTLNotSupported is returned by Cache.Set when the backend cannot honor
// the requested ttl semantics, such as a per-entry lifetime on a backend
// without per-entry expiration.
var ErrTTLNotSupported = errors.New("cache backend does not support the requested ttl")

// Cache provides a typed key/value cache abstraction.
//
// Type Parameters:
//   - T: Cached value type
//
// Error Handling:
//   - Get returns ErrEntryNotFound when the key does not exist.
//   - Delete is idempotent: deleting a missing key returns nil.
//   - Set returns ErrTTLNotSupported when the backend cannot honor the
//     requested ttl semantics.
//
// TTL Semantics:
//   - ttl == 0 means the entry never expires.
//   - ttl > 0 sets a per-entry lifetime; backends that cannot honor it must
//     return ErrTTLNotSupported instead of silently mis-honoring the request.
//   - ttl < 0 is invalid and returns an error.
//
// The ctx parameter carries cancellation and tracing for backends that talk
// to remote systems; in-memory backends may ignore it. A nil ctx is treated
// as context.Background(), so callers never need to normalize it themselves.
type Cache[T any] interface {
	// Get retrieves a value from the cache by key.
	// Returns ErrEntryNotFound if the key does not exist.
	Get(ctx context.Context, key string) (T, error)

	// Set stores a value in the cache with the specified TTL.
	Set(ctx context.Context, key string, value T, ttl time.Duration) error

	// Delete removes a key from the cache.
	// Deleting a key that does not exist is not an error.
	Delete(ctx context.Context, key string) error

	// Exists checks if a key exists in the cache.
	// Returns true if the key exists, false otherwise.
	Exists(ctx context.Context, key string) bool
}

// DistributedCache extends Cache with explicit local-plus-remote synchronization helpers.
//
// Type Parameters:
//   - T: Cached value type
type DistributedCache[T any] interface {
	Cache[T]

	// SetWithSync stores a value in both local and distributed cache with synchronization.
	SetWithSync(ctx context.Context, key string, value T, localTTL time.Duration, remoteTTL time.Duration) error

	// GetWithSync retrieves a value from local cache first, then from distributed cache if not found.
	GetWithSync(ctx context.Context, key string, localTTL time.Duration) (T, error)

	// DeleteWithSync removes a value from both local and distributed cache with synchronization.
	DeleteWithSync(ctx context.Context, key string) error
}
