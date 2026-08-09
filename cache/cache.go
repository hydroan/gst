// Package cache is the public facade of the framework's in-memory cache.
//
// The concrete implementations live under internal/cache and are not part of
// the public API: business projects import this package only, so their builds
// link just the forwarded backend below instead of every cache library the
// framework ships. Every backend carries built-in defaults; there is nothing
// to configure or initialize. The one available knob is cache.max_entries,
// which overrides the per-type entry bound.
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
// | otter       | Per-entry expiration      |
//
// The forwarded backend is ccache. The selection criteria, in priority order:
//
//  1. Writes must be visible: a value stored by Set is readable by the next
//     Get. This is what disqualifies ristretto, whose admission policy
//     silently rejects new keys once the resident set is warm — Set reports
//     success and the entry is never stored.
//  2. Per-entry TTL, so callers control each entry's lifetime.
//  3. A capacity bound that actually evicts, so an attacker-influenced key
//     space cannot grow without limit.
//
// Write honesty and scan resistance turn out to be the same dial. Measured on
// a 200-key hot set in a 100k-entry cache swept by 100k fresh keys, ristretto
// keeps all 200 and loses most writes, ccache keeps none and loses no write,
// and otter keeps 198 and loses no write. otter is therefore the stronger
// candidate on paper and is carried in internal/cache for that reason; it is
// not forwarded yet because its upstream has been dormant for months, a risk
// that only matters once every build links it. Revisiting that is a one-line
// change here, and not a knob for business projects.
package cache

import (
	"github.com/hydroan/gst/internal/cache/ccache"
	"github.com/hydroan/gst/types"
)

// Cache returns the process-wide typed cache backed by the forwarded backend.
func Cache[T any]() types.Cache[T] {
	return ccache.Cache[T]()
}
