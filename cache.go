package gst

import (
	"github.com/hydroan/gst/internal/types"
)

// ErrEntryNotFound is returned when a cache entry is not found.
var ErrEntryNotFound = types.ErrEntryNotFound

// ErrTTLNotSupported is returned by Cache.Set when the backend cannot honor
// the requested ttl semantics, such as a per-entry lifetime on a backend
// without per-entry expiration.
var ErrTTLNotSupported = types.ErrTTLNotSupported

// Cache provides a typed key/value cache abstraction.
type Cache[T any] = types.Cache[T]
