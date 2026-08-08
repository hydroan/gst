// Package dcache provides a two-level cache: a per-process in-memory tier and
// an optional Redis tier kept in sync across instances through Kafka events.
//
// Choosing an entry point:
//   - NewLocalCache: the cache lives only in the current process
//     (Set/Get/Delete).
//   - NewDistributedCache without the WithSync methods: state propagates to
//     the local tier of every instance through Kafka (Set/Get/Delete).
//   - NewDistributedCache with the WithSync methods: state additionally
//     persists in Redis (SetWithSync/GetWithSync/DeleteWithSync).
//
// # Known Limitations
//
// These are accepted trade-offs of the current design, documented so callers
// can judge whether they fit their workload:
//
//   - Consumers join Kafka with a fresh group id at the end of the topic.
//     Events published while a consumer (an instance or the state node) is
//     down are never replayed for it; the Redis tier misses those writes
//     until the same key is written again.
//   - Cross-host ordering is decided by producer UnixNano timestamps. Clock
//     skew between hosts can drop a legitimately newer write as stale.
//   - The producer fails fast (a 300ms retry budget). A Kafka outage longer
//     than that drops the event with only a log line, leaving instance
//     caches inconsistent until the key is written again.
//   - The state node bounds its per-key timestamp table by LRU eviction;
//     evicting a key reopens that key's out-of-order acceptance window.
//   - There is no lifecycle API: producers, consumers, goroutine pools and
//     monitor tickers live for the whole process lifetime.
package dcache
