// Package dcache provides a replicated in-memory cache: a per-process store
// whose set and delete operations propagate to the store of every other
// instance through Kafka events. A cache that lives only in the current
// process is the cache package instead; this one is for state every
// instance must see.
//
// There is no shared storage tier. Entries must be rebuildable from their
// source of truth on a miss (cache-aside), and a bounded TTL is the only
// backstop that reconciles instances after a lost event: do not rely on
// ttl == 0 entries staying consistent across instances.
//
// # Known Limitations
//
// These are accepted trade-offs of the current design, documented so callers
// can judge whether they fit their workload:
//
//   - Values travel between instances as JSON. T must round-trip through
//     encoding/json without loss: a value whose state lives in unexported
//     fields is stored intact locally but arrives at every peer as a hollow
//     decoded copy, and nothing reports it. A marshal failure, by contrast,
//     is returned by Set.
//   - Consumers join Kafka with a fresh group id at the end of the topic.
//     Events published while an instance is down are never replayed for it;
//     its store misses those writes until the same key is written again or
//     the TTL expires.
//   - Cross-host ordering is decided by producer UnixNano timestamps. Clock
//     skew between hosts can drop a legitimately newer write as stale.
//   - Delivery is best-effort and bounded: a record that cannot be
//     delivered within its budget (a few retries inside a few seconds),
//     that exceeds the broker's message size limit, or that arrives while
//     the publishing pool is saturated is dropped with an error log,
//     leaving the peers behind until the key is written again or the TTL
//     expires.
//   - A peer stores an entry with the ttl the event carries, counted from
//     the moment the event is applied, so the entry lives on the peer for
//     the propagation delay longer than on the writer.
//   - Each instance bounds its per-key timestamp table by LRU eviction;
//     evicting a key reopens that key's out-of-order acceptance window.
//   - There is no lifecycle API: producers, consumers, goroutine pools and
//     monitor tickers live for the whole process lifetime.
package dcache
