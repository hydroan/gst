// Package cache is the public facade of the framework's in-memory cache.
//
// The concrete implementations live under internal/cache and are not part of
// the public API: business projects import this package only, so their builds
// link just the forwarded backend below instead of every cache library the
// framework ships. Every backend carries built-in defaults; there is nothing
// to configure or initialize.
//
// # Available Backends (internal/cache)
//
// | Package     | Expiration Strategy       |
// |-------------|---------------------------|
// | lru         | No expiration             |
// | cmap        | No expiration             |
// | smap        | No expiration             |
// | fastcache   | No expiration             |
// | lrue        | Global expiration         |
// | bigcache    | Global expiration         |
// | ristretto   | Per-entry expiration      |
// | freecache   | Per-entry expiration      |
// | ccache      | Per-entry expiration      |
// | gocache     | Per-entry expiration      |
//
// The forwarded backend is ristretto: it is the only in-memory backend with
// per-entry TTL, bounded capacity and scan-resistant admission at the same
// time. Switching the recommendation is a framework decision made here, not a
// knob exposed to business projects.
package cache

import (
	"github.com/hydroan/gst/internal/cache/ristretto"
	"github.com/hydroan/gst/types"
)

// Cache returns the process-wide typed cache backed by the forwarded backend.
func Cache[T any]() types.Cache[T] {
	return ristretto.Cache[T]()
}
