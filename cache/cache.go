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
// | Package     | Expiration Strategy  | Storage        |
// |-------------|----------------------|----------------|
// | lru         | No expiration        | Live values    |
// | cmap        | No expiration        | Live values    |
// | smap        | No expiration        | Live values    |
// | freelru     | Per-entry expiration | Live values    |
// | ccache      | Per-entry expiration | Live values    |
// | otter       | Per-entry expiration | Live values    |
// | ristretto   | Per-entry expiration | Live values    |
// | fastcache   | No expiration        | Serialized     |
// | bigcache    | No expiration        | Serialized     |
// | freecache   | Per-entry expiration | Serialized     |
//
// A backend that cannot honor a lifetime rejects it with ErrTTLNotSupported
// rather than storing the entry on different terms. That is why the "No
// expiration" backends still accept ttl 0 — for them it is the truth — and
// why none of them is configured with a shorter global window that would
// quietly turn "never expires" into ten minutes.
//
// The Storage column is a correctness property, not a performance note. A
// live-value backend hands back the object that was stored, so any T works.
// A serialized backend encodes each value on the way in — scalars and byte
// slices through a compact representation, everything else through JSON — and
// so silently drops whatever the encoding cannot carry: unexported fields
// vanish without an error, and a type whose behavior lives in them comes back
// inert. Caching a *rate.Limiter through one encodes to "{}" and decodes to a
// limiter with a zero burst that denies every request; the failure surfaces
// far from the cache, as a program that still runs but has quietly stopped
// working. Serialized backends are for plain data transfer objects only,
// which is also why none of them is a candidate for the forwarded default.
// The same caveat applies to the redis cache outside this package.
//
// The forwarded backend is freelru. The selection criteria, in priority order:
//
//  1. Writes must land and stay. A value stored by Set has to be readable by
//     the next Get, and still be there when a later request comes looking.
//     Both halves matter and they fail separately: ristretto rejects new keys
//     outright once the resident set is warm, so Set reports success and the
//     entry never exists; otter accepts them and then evicts them first,
//     because a fresh key holds the lowest frequency estimate in the cache.
//     Measured on a warm 100k-entry cache taking 10k new keys, ristretto lost
//     99% immediately, otter lost none immediately and 44% within 200ms, and
//     freelru and ccache lost none at either point.
//  2. Per-entry TTL, so callers control each entry's lifetime.
//  3. A capacity bound that actually evicts, so an attacker-influenced key
//     space cannot grow without limit.
//
// Retaining new writes and resisting scans are the same dial turned opposite
// ways, so criterion 1 is bought by giving something up. Swept by 100k fresh
// keys, a 200-key hot set in a 100k-entry cache survives whole under
// ristretto, largely under otter, and not at all under the plain-LRU
// backends — the same ordering as the write-loss numbers above, reversed. A
// framework cache is written to and read back far more often than it is
// swept, so this package pays for writes and forgoes scan resistance.
//
// freelru wins over the other backend that meets all three criteria: it
// shards its lock domain per core instead of funneling writes through one
// worker goroutine, which under a 90/10 read-write mix on 14 cores is 27 ns
// against ccache's 385 ns, and it allocates on neither path. A workload that
// needs scan resistance instead should reach for a backend that offers it
// rather than expect this default to cover both; the choice is made here and
// is not a knob for business projects.
package cache

import (
	"github.com/hydroan/gst/internal/cache/freelru"
	"github.com/hydroan/gst/types"
)

// Cache returns the process-wide typed cache backed by the forwarded backend.
func Cache[T any]() types.Cache[T] {
	return freelru.Cache[T]()
}
