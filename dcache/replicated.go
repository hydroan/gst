package dcache

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/cockroachdb/errors"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/hydroan/gst/cache"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/capacity"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"github.com/panjf2000/ants/v2"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

const (
	compKey = "comp"

	// minGoroutines is the floor of the event-publishing pool capacity.
	minGoroutines = 10000
)

// watermarkEntries bounds the per-key timestamp table: an order of magnitude
// above the store's entry bound, because the watermark must keep covering
// keys the store has already evicted or expired — a stale peer event could
// otherwise resurrect them — and capped so a huge store configuration cannot
// balloon the table, which exists once per cached type.
func watermarkEntries() int {
	return min(10*capacity.Entries(), 10_000_000)
}

const (
	opSet op = iota
	opDel
)

var (
	// caches keeps one instance per value type; see the registry
	// package for the lookup and double-checked creation semantics.
	caches = registry.New()

	_ types.Cache[any] = (*replicatedCache[any])(nil)
)

type op int

func (o op) String() string {
	switch o {
	case opSet:
		return "set"
	case opDel:
		return "del"
	default:
		return "unknown"
	}
}

type event struct {
	CacheID string `json:"cache_id"`

	Key string          `json:"key"` // cache key
	TS  int64           `json:"ts"`  // unix nanoseconds
	Op  op              `json:"op"`
	Val json.RawMessage `json:"val"`
	Typ string          `json:"typ"`
	raw any
	TTL time.Duration `json:"ttl"`

	Hostname string `json:"hostname"` // which server produced the event
}

// eventLogView is the lazy, bounded log form of an event. It is a separate
// type because the truncation must never reach the wire: event itself
// marshals verbatim into the kafka payload, while the log encoder marshals
// this wrapper — and only when the entry is actually written, so a disabled
// log level costs no more than boxing one pointer.
type eventLogView struct {
	e *event
}

// MarshalJSON renders every field verbatim except the payload, which is
// truncated so a large cached value cannot flood a log line — the collapsing
// encoder has no truncation of its own.
func (v eventLogView) MarshalJSON() ([]byte, error) {
	if v.e == nil {
		return []byte("null"), nil
	}

	const maxLoggedValue = 1024
	val := v.e.Val
	if len(val) > maxLoggedValue {
		val = val[:maxLoggedValue]
	}
	// A zero ttl means "never expires" in the cache contract; rendering it
	// as a zero duration would read as "expired on arrival".
	ttl := "never"
	if v.e.TTL != 0 {
		ttl = util.FormatDurationSmart(v.e.TTL, 2)
	}
	return json.Marshal(map[string]any{
		"ts":       time.Unix(0, v.e.TS).UTC().Format(consts.LayoutTimeEncoder),
		"cache_id": v.e.CacheID,
		"hostname": v.e.Hostname,
		"typ":      v.e.Typ,
		"op":       v.e.Op.String(),
		"key":      v.e.Key,
		"value":    string(val),
		"ttl":      ttl,
	})
}

// Cache returns the replicated cache of type T, creating it on first use.
//
// Why keep one cache per type in a registry?
// Each type's cache owns the goroutine that watches the peer set and delete events, and they
// never interfere with each other. The number of types is bounded, so only a few goroutines ever
// listen for kafka events, and fewer listeners means better throughput.
//
// Without the registry, every Cache call would spawn another goroutine listening
// for kafka events and therefore far too many kafka consumers, which is not what we want. Since
// this function is exported we cannot stop other developers from calling it repeatedly, so the
// control has to stay here.
//
// The arithmetic:
//
//	kafka consumers on one node: number of service processes * number of dcache instances,
//	and normally only one service process runs.
//	total kafka listeners: consumers per node * number of nodes
func Cache[T any]() (types.Cache[T], error) {
	return registry.LoadE(caches, func() (types.Cache[T], error) {
		// Fail fast on an environment where the cache could never replicate,
		// instead of handing out an instance that silently degrades to a
		// process-local cache. Both conditions are re-checked on the next
		// call: a failed construction is not cached.
		if logger.Dcache == nil {
			return nil, errors.New("the logging setup has not run yet, dcache requires it")
		}
		if !config.App.Kafka.Enabled {
			return nil, errors.New("kafka is not enabled, the replicated cache requires it")
		}
		return newReplicatedCache[T](cache.Cache[entry[T]]())
	})
}

// typeName returns a name that identifies T uniquely across packages.
//
// reflect's Name is empty for unnamed types such as pointers, slices and
// maps, and its String uses short package names, so two same-named types from
// different packages render alike. Pairing the package path with the full
// string keeps both cases apart.
func typeName[T any]() string {
	typ := reflect.TypeFor[T]()
	return typ.PkgPath() + "|" + typ.String()
}

// entry wraps the cached value so the store below is keyed by a type of
// dcache's own. cache.Cache keeps one process-wide instance per value type,
// and the wrapper gives dcache a registry slot — and so a key space — that
// business code holding cache.Cache[T]() can never share or collide with,
// while the store still follows whatever backend the cache facade forwards
// to.
type entry[T any] struct{ Value T }

// replicatedCache implements types.Cache by replication. Every instance owns
// a process-local tier and mirrors its set and delete operations to the
// local tier of every other instance through kafka events:
//   - Local memory cache for high-speed access.
//   - Kafka for cross-instance propagation of set and delete operations.
//
// There is no shared storage tier; see the package documentation for the
// consistency trade-offs. Performance metrics are tracked (hits/misses) and
// a controlled goroutine pool handles event publishing.
type replicatedCache[T any] struct {
	// store is the process-local tier the instance reads from and applies
	// peer events to.
	store types.Cache[entry[T]]

	// typ is the type of the replicated cache.
	// When an instance receives a peer event it compares event.Typ with its own
	// type and ignores the event when they differ.
	// NOTE: the typ of the replicated cache instances of one type is always the same.
	typ string

	// cacheID identifies one replicated cache instance, and every instance has its own.
	// When an instance receives a peer event it compares event.CacheID with its
	// own ID and ignores the event when they are equal, because it published that event itself.
	// NOTE: the cacheID of two replicated cache instances is never the same.
	cacheID  string
	hostname string

	// stats
	hits           atomic.Int64
	misses         atomic.Int64
	sets           atomic.Int64
	deletes        atomic.Int64
	peerSets       atomic.Int64
	peerDeletes    atomic.Int64
	publishDropped atomic.Int64
	publishFailed  atomic.Int64

	// topic carries the set and delete events of every instance; it is
	// resolved once at construction and the default derives from the
	// application name.
	topic string
	// appliedTS is the highest applied timestamp per key, fed by local
	// writes and accepted peer events alike, so a stale peer event can
	// neither overwrite a newer local write nor resurrect a locally deleted
	// key. The partition key keeps one key's events ordered within a
	// partition, but a partition count change or a replay can still deliver
	// an older event afterwards. Bounded by LRU: evicting a key only
	// reopens that key's out-of-order window.
	appliedTS *lru.Cache[string, int64]
	// watermarkMu guards the compound invariant that a store write and its
	// watermark entry happen as one atomic step, on the local write path and
	// the peer apply path alike; without it a peer event that passed its
	// staleness check could land on top of a newer concurrent local write,
	// with the advanced watermark then blocking every later correction. A
	// single mutex is deliberate: every write already pays two JSON marshals
	// and a pool submit, so the lock is not the bottleneck; shard it by key
	// only if profiles ever say otherwise.
	watermarkMu sync.Mutex

	// pub publishes this instance's set and delete events; sub receives the
	// events of every instance, including this one's own, which are skipped
	// by cacheID.
	pub *kgo.Client
	sub *kgo.Client

	// logger is the cache internal logger.
	logger types.Logger

	// pubPool bounds the goroutines publishing events.
	pubPool *ants.Pool
}

// newReplicatedCache creates and initializes one replicated cache instance
// around the given store and a kafka producer/consumer pair. The store is a
// parameter so tests can watch propagation between two instances holding
// separate stores.
func newReplicatedCache[T any](store types.Cache[entry[T]]) (*replicatedCache[T], error) {
	cacheID := uuid.NewV7()
	hostname, err := os.Hostname()
	if err != nil {
		// First-hand exit of a stack-less third-party/standard-library error;
		// see the error-stack contract in the database package doc.
		return nil, errors.WithStack(err)
	}

	dc := &replicatedCache[T]{
		store:    store,
		cacheID:  cacheID.String(),
		typ:      typeName[T](),
		hostname: hostname,
	}
	// comp marks the replicated cache name so its log lines are searchable.
	comp := fmt.Sprintf("[%s:dcache:%s]", hostname, reflect.TypeFor[T]().String())
	dc.logger = logger.Dcache.With("hostname", hostname, compKey, comp)

	// Undo the partially built kafka clients when a later step fails: the
	// registry does not cache failed constructions, so a retried call would
	// otherwise leak another set of connections and goroutines each time.
	succeeded := false
	defer func() {
		if succeeded {
			return
		}
		if dc.pub != nil {
			dc.pub.Close()
		}
		if dc.sub != nil {
			dc.sub.Close()
		}
	}()

	if dc.appliedTS, err = lru.New[string, int64](watermarkEntries()); err != nil {
		return nil, errors.WithStack(err)
	}

	// setup kafka; the consumer group name follows the topic, and the group
	// suffix is the instance's own id so two instances can never share a
	// group and split the partitions between them.
	dc.topic = cacheTopic()
	cfg := config.App.Kafka
	if dc.pub, err = newProducer(cfg, dc.topic); err != nil {
		return nil, err
	}
	if dc.sub, err = newConsumer(cfg, dc.topic, dc.topic+"-"+dc.cacheID); err != nil {
		return nil, err
	}

	// setup the goroutine pool: the heuristic capacity is raised to the
	// floor, which matters on a small machine. Nonblocking keeps a kafka
	// outage from propagating backpressure into Set/Delete callers: when
	// every worker is stuck producing, a new event is dropped with a log
	// line instead of blocking the write path.
	gocap := max(runtime.NumCPU()*2000, minGoroutines)
	pool, err := ants.NewPool(gocap, ants.WithPreAlloc(false), ants.WithNonblocking(true),
		// Publishing tasks panic into the component logger, matching
		// logPanic; the pool's default handler would print to stderr, out of
		// the collected log stream.
		ants.WithPanicHandler(func(r any) {
			dc.logger.Errorz(
				"publishing task panicked",
				zap.Any("panic", r),
				zap.ByteString("stack", debug.Stack()),
			)
		}))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	dc.pubPool = pool

	dc.listenEvents()
	dc.startMonitor()

	succeeded = true
	return dc, nil
}

// Set sets a key-value pair in the store and publishes an opSet event that
// carries the value to the store of every other instance.
//
// An error can arrive after the local store already holds the change: it
// then reports a failed publication, not a failed write — the peers will not
// see the value. A nil return does not guarantee peer delivery either; the
// package documentation lists the drop cases of the best-effort broadcast.
func (dc *replicatedCache[T]) Set(ctx context.Context, key string, value T, ttl time.Duration) (err error) {
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	if err = validKey(key); err != nil {
		return err
	}

	// The write and its watermark entry are one critical section shared with
	// the peer apply path, so an older peer event that already passed its
	// staleness check cannot land on top of this newer write. Writing the
	// store first and reporting its failure also keeps the peers from
	// holding a value this instance does not have.
	dc.watermarkMu.Lock()
	ts := time.Now().UnixNano()
	if err = dc.store.Set(ctx, key, entry[T]{Value: value}, ttl); err != nil {
		dc.watermarkMu.Unlock()
		return err
	}
	dc.advanceWatermarkLocked(key, ts)
	dc.watermarkMu.Unlock()
	dc.sets.Add(1)

	// A publication error reaches the caller: the local tier already holds
	// the value, but the peers never will.
	return dc.sendEvent(&event{
		TS:  ts,
		Op:  opSet,
		Key: key,
		raw: value,
		TTL: ttl,
	})
}

// validKey rejects a key JSON would silently rewrite: encoding/json replaces
// invalid UTF-8 with U+FFFD, so the peers would apply the operation to a
// different key than the local store did.
func validKey(key string) error {
	if !utf8.ValidString(key) {
		return errors.New("the key is not valid UTF-8 and would reach the peers rewritten")
	}
	return nil
}

func (dc *replicatedCache[T]) Get(ctx context.Context, key string) (value T, err error) {
	stored, err := dc.store.Get(ctx, key)
	if err == nil {
		dc.hits.Add(1)
		return stored.Value, nil
	}
	var zero T
	if errors.Is(err, types.ErrEntryNotFound) {
		dc.misses.Add(1)
		return zero, types.ErrEntryNotFound
	}

	dc.logger.Warnz("failed to get from the store", zap.Error(err))
	return zero, err
}

// Delete removes the entry from the store and publishes an opDel event that
// removes it from the store of every other instance. See Set for the
// semantics of the returned error.
func (dc *replicatedCache[T]) Delete(ctx context.Context, key string) (err error) {
	if err = validKey(key); err != nil {
		return err
	}
	// See Set: the delete and its watermark entry are one critical section,
	// so a stale peer set cannot resurrect the key this instance removed.
	dc.watermarkMu.Lock()
	ts := time.Now().UnixNano()
	if err = dc.store.Delete(ctx, key); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
		dc.watermarkMu.Unlock()
		return err
	}
	dc.advanceWatermarkLocked(key, ts)
	dc.watermarkMu.Unlock()
	dc.deletes.Add(1)

	return dc.sendEvent(&event{
		TS:  ts,
		Op:  opDel,
		Key: key,
	})
}

func (dc *replicatedCache[T]) Exists(ctx context.Context, key string) bool {
	return dc.store.Exists(ctx, key)
}

// logPanic recovers a panic that would otherwise kill the process and turns
// it into a structured error record. The goroutine still ends: for the event
// listener that means the instance stops receiving peer events and its store
// serves stale entries until their TTLs expire, while its own writes keep
// publishing. Restarting is a lifecycle this package deliberately does not
// have.
func (dc *replicatedCache[T]) logPanic(goroutine string) {
	if r := recover(); r != nil {
		dc.logger.Errorz(
			"goroutine panicked",
			zap.String("goroutine", goroutine),
			zap.Any("panic", r),
			zap.ByteString("stack", debug.Stack()),
		)
	}
}

// listenEvents consumes the set and delete events published by the other
// instances and applies them to the local tier.
func (dc *replicatedCache[T]) listenEvents() {
	go func() {
		defer dc.logPanic("replicatedCache.listenEvents")

		for {
			fetches := dc.sub.PollFetches(context.Background())
			if fetches.IsClientClosed() {
				// A closed client cannot recover here; stop the listener
				// instead of spinning on an immediately returning poll.
				dc.logger.Error("kafka consumer client closed, stopping the event listener")
				return
			}
			fetches.EachError(func(s string, i int32, err error) {
				dc.logger.Errorz(
					"failed to fetch from kafka",
					zap.Error(err),
					zap.String("topic", dc.topic),
					zap.String("s", s),
					zap.Int32("i", i),
				)
			})
			records := fetches.Records()
			if len(records) == 0 {
				continue
			}
			for _, record := range records {
				evt := new(event)
				if err := json.Unmarshal(record.Value, evt); err != nil {
					dc.logger.Errorz(
						"failed to unmarshal event",
						zap.Error(err),
						zap.String("topic", dc.topic),
						zap.ByteString("value", record.Value),
					)
					continue
				}
				// Skip the events this instance published itself: publishing
				// already applied the operation to its local tier. Check the
				// cache ID first, after which the cache type barely needs
				// checking.
				if evt.CacheID == dc.cacheID {
					continue
				}
				// The topic is shared by the caches of every type, so foreign
				// types are filtered out here. Local stores are one per type,
				// which is what keeps one key from colliding across types.
				// The filter must also run before the watermark checks:
				// appliedTS is keyed by the bare cache key, and recording a
				// foreign type's event would advance the watermark of this
				// type's same-named key.
				if evt.Typ != dc.typ {
					continue
				}
				dc.logger.Debugz("consume event", zap.Any("event", eventLogView{evt}))
				switch evt.Op {
				case opSet:
					var val T
					if err := json.Unmarshal(evt.Val, &val); err != nil {
						dc.logger.Errorz("failed to unmarshal event value", zap.Error(err), zap.Any("event", eventLogView{evt}))
						continue
					}
					stale, err := dc.applyPeerSet(evt, val)
					if err != nil {
						dc.logger.Warnz("failed to set to the store", zap.Error(err))
						continue
					}
					if stale {
						dc.logger.Debugz("skipping a stale peer event superseded by a newer write", zap.Any("event", eventLogView{evt}))
						continue
					}
					dc.peerSets.Add(1)
				case opDel:
					stale, err := dc.applyPeerDelete(evt)
					if err != nil {
						dc.logger.Warnz("failed to delete from the store", zap.Error(err))
						continue
					}
					if stale {
						dc.logger.Debugz("skipping a stale peer event superseded by a newer write", zap.Any("event", eventLogView{evt}))
						continue
					}
					dc.peerDeletes.Add(1)
				default:
					dc.logger.Warnz("unknown event op", zap.Any("event", eventLogView{evt}))
				}
			}
		}
	}()
}

// applyPeerSet applies a peer set event to the store when it is newer than
// its key's watermark. The staleness check, the store write and the
// watermark advance are one critical section shared with the local write
// path — a stale set after a delete would otherwise bring the entry back —
// while unmarshaling stays outside of it. A failed store write does not
// claim the event's timestamp, so a redelivery can still apply. It reports
// stale=true when a newer write already claimed the key.
func (dc *replicatedCache[T]) applyPeerSet(evt *event, val T) (stale bool, err error) {
	dc.watermarkMu.Lock()
	defer dc.watermarkMu.Unlock()
	if last, ok := dc.appliedTS.Get(evt.Key); ok && evt.TS <= last {
		return true, nil
	}
	if err := dc.store.Set(context.Background(), evt.Key, entry[T]{Value: val}, evt.TTL); err != nil {
		return false, err
	}
	dc.appliedTS.Add(evt.Key, evt.TS)
	return false, nil
}

// applyPeerDelete removes the event's key from the store when the event is
// newer than the key's watermark; see applyPeerSet for the locking.
func (dc *replicatedCache[T]) applyPeerDelete(evt *event) (stale bool, err error) {
	dc.watermarkMu.Lock()
	defer dc.watermarkMu.Unlock()
	if last, ok := dc.appliedTS.Get(evt.Key); ok && evt.TS <= last {
		return true, nil
	}
	if err := dc.store.Delete(context.Background(), evt.Key); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
		return false, err
	}
	dc.appliedTS.Add(evt.Key, evt.TS)
	return false, nil
}

// advanceWatermarkLocked records ts as the newest applied timestamp of key
// when it is newer than the recorded one. The caller holds watermarkMu; the
// monotonic check keeps a clock anomaly from regressing the watermark.
func (dc *replicatedCache[T]) advanceWatermarkLocked(key string, ts int64) {
	if last, ok := dc.appliedTS.Get(key); ok && ts <= last {
		return
	}
	dc.appliedTS.Add(key, ts)
}

// sendEvent publishes a cache event to the kafka topic through the bounded
// goroutine pool. A marshal failure is returned to the caller: the value is
// already stored locally but can never reach the peers, and that must not
// stay silent. A full pool drops the event with a log line instead of
// blocking the caller, matching the documented best-effort delivery.
func (dc *replicatedCache[T]) sendEvent(evt *event) error {
	if evt == nil {
		return nil
	}
	evt.CacheID = dc.cacheID
	evt.Typ = dc.typ
	evt.Hostname = dc.hostname

	// Marshal the caller's value synchronously: doing it in the pool goroutine
	// would keep reading the value concurrently after Set has returned.
	val, err := json.Marshal(evt.raw)
	if err != nil {
		return errors.Wrap(err, "marshal the cached value for publication")
	}
	evt.Val = val
	// Drop the reference to the caller's value: it does not marshal into the
	// event (raw is unexported), but keeping it would pin the value while the
	// event queues in the pool.
	evt.raw = nil
	data, err := json.Marshal(evt)
	if err != nil {
		return errors.Wrap(err, "marshal the cache event")
	}
	err = dc.pubPool.Submit(func() {
		record := &kgo.Record{
			Topic: dc.topic,
			// The key pins every event for one cache key to one partition,
			// so every instance observes one key's events in one shared
			// order. Correctness against reordering rests on the timestamp
			// watermark; the pinning just keeps the common path free of
			// out-of-order rejections.
			Key:   []byte(evt.Key),
			Value: data,
		}
		dc.logger.Debugz("publish event", zap.Any("event", eventLogView{evt}))
		if pubErr := dc.pub.ProduceSync(context.Background(), record).FirstErr(); pubErr != nil {
			dc.publishFailed.Add(1)
			dc.logger.Errorz("failed to publish event", zap.Error(pubErr), zap.Any("event", eventLogView{evt}))
		}
	})
	if err != nil {
		dc.publishDropped.Add(1)
		dc.logger.Errorz("event dropped: failed to submit it to the publishing pool", zap.Error(err), zap.Any("event", eventLogView{evt}))
	}
	return nil
}

func (dc *replicatedCache[T]) startMonitor() {
	ticker := time.NewTicker(3 * time.Minute)
	go func() {
		defer dc.logPanic("replicatedCache.startMonitor")
		for range ticker.C {
			if flag.Lookup("test.v") == nil {
				dc.logger.Infoz("cache metrics", zap.Any("metrics", dc.metrics()))
			}
		}
	}()
}

// metrics snapshots the instance counters for the monitor. It is unexported:
// the exported surface is types.Cache[T], which has no metrics channel.
func (dc *replicatedCache[T]) metrics() *replicatedMetrics {
	return &replicatedMetrics{
		Hits:    dc.hits.Load(),
		Misses:  dc.misses.Load(),
		Ratio:   hitRatio(dc.hits.Load(), dc.misses.Load()),
		Sets:    dc.sets.Load(),
		Deletes: dc.deletes.Load(),

		PeerSets:    dc.peerSets.Load(),
		PeerDeletes: dc.peerDeletes.Load(),

		PublishDropped: dc.publishDropped.Load(),
		PublishFailed:  dc.publishFailed.Load(),

		GoroutinesPoolCap: int64(dc.pubPool.Cap()),
		GoroutinesUsed:    int64(dc.pubPool.Running()),
	}
}

// replicatedMetrics is the monitor snapshot of one instance's counters. The
// peer counters record how many peer events this instance applied to its
// store; the publish counters record broadcast events lost to a saturated
// pool and to failed produces.
type replicatedMetrics struct {
	Hits    int64 `json:"hits"`
	Misses  int64 `json:"misses"`
	Ratio   int64 `json:"ratio"`
	Sets    int64 `json:"sets"`
	Deletes int64 `json:"deletes"`

	PeerSets    int64 `json:"peer_sets"`
	PeerDeletes int64 `json:"peer_deletes"`

	PublishDropped int64 `json:"publish_dropped"`
	PublishFailed  int64 `json:"publish_failed"`

	GoroutinesPoolCap int64 `json:"goroutines_pool_cap"`
	GoroutinesUsed    int64 `json:"goroutines_used"`
}
