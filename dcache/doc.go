// Package dcache provides a replicated in-memory cache: a per-process local
// tier whose set and delete operations propagate to the local tier of every
// other instance through Kafka events.
//
// Choosing an entry point:
//   - NewLocalCache: the cache lives only in the current process.
//   - NewDistributedCache: every instance keeps its own local tier and
//     mirrors set/delete operations to the local tier of every peer.
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
//   - Consumers join Kafka with a fresh group id at the end of the topic.
//     Events published while an instance is down are never replayed for it;
//     its local tier misses those writes until the same key is written
//     again or the TTL expires.
//   - Cross-host ordering is decided by producer UnixNano timestamps. Clock
//     skew between hosts can drop a legitimately newer write as stale.
//   - The producer fails fast (a 300ms retry budget). A Kafka outage longer
//     than that drops the event with only a log line, leaving instance
//     caches inconsistent until the key is written again or the TTL
//     expires.
//   - Each instance bounds its per-key timestamp table by LRU eviction;
//     evicting a key reopens that key's out-of-order acceptance window.
//   - There is no lifecycle API: producers, consumers, goroutine pools and
//     monitor tickers live for the whole process lifetime.
package dcache
