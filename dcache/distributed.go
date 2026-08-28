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
	"uuid"

	"github.com/cockroachdb/errors"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/hydroan/gst/cache"
	"github.com/hydroan/gst/config"
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
)

const (
	opSet op = iota
	opDel
)

var (
	// distributedCaches keeps one instance per value type; see the registry
	// package for the lookup and double-checked creation semantics.
	distributedCaches = registry.New()

	_ types.Cache[any] = (*distributedCache[any])(nil)
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

// logView returns the bounded log form of the event: every field verbatim
// except the payload, which is truncated so a large cached value cannot
// flood a log line — the collapsing encoder has no truncation of its own.
func (e *event) logView() map[string]any {
	if e == nil {
		return nil
	}

	const maxLoggedValue = 1024
	val := e.Val
	if len(val) > maxLoggedValue {
		val = val[:maxLoggedValue]
	}
	return map[string]any{
		"ts":       time.Unix(0, e.TS).UTC().Format(consts.LayoutTimeEncoder),
		"cache_id": e.CacheID,
		"hostname": e.Hostname,
		"typ":      e.Typ,
		"op":       e.Op.String(),
		"key":      e.Key,
		"value":    string(val),
		"ttl":      util.FormatDurationSmart(e.TTL, 2),
	}
}

// NewDistributedCache returns the distributed cache of type T, creating it on first use.
//
// Why keep one cache per type in a registry?
// Each type's cache owns the goroutine that watches the peer set and delete events, and they
// never interfere with each other. The number of types is bounded, so only a few goroutines ever
// listen for kafka events, and fewer listeners means better throughput.
//
// Without the registry, every NewDistributedCache call would spawn another goroutine listening
// for kafka events and therefore far too many kafka consumers, which is not what we want. Since
// this function is exported we cannot stop other developers from calling it repeatedly, so the
// control has to stay here.
//
// The arithmetic:
//
//	kafka consumers on one node: number of service processes * number of DistributedCache instances,
//	and normally only one service process runs.
//	total kafka listeners: consumers per node * number of nodes
func NewDistributedCache[T any]() (types.Cache[T], error) {
	return registry.LoadE(distributedCaches, func() (types.Cache[T], error) {
		return newDistributedCache[T](cache.Cache[entry[T]]())
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

// distributedCache is a replicated in-memory cache. Every instance owns a
// process-local tier and mirrors its set and delete operations to the local
// tier of every other instance through kafka events:
//   - Local memory cache for high-speed access.
//   - Kafka for cross-instance propagation of set and delete operations.
//
// There is no shared storage tier; see the package documentation for the
// consistency trade-offs. Performance metrics are tracked (hits/misses) and
// a controlled goroutine pool handles event publishing.
type distributedCache[T any] struct {
	// store is the process-local tier the instance reads from and applies
	// peer events to.
	store types.Cache[entry[T]]

	// typ is the type of the distributed cache.
	// When an instance receives a peer event it compares event.Typ with its own
	// type and ignores the event when they differ.
	// NOTE: the typ of the distributed cache instances of one type is always the same.
	typ string

	// cacheID identifies one distributed cache instance, and every instance has its own.
	// When an instance receives a peer event it compares event.CacheID with its
	// own ID and ignores the event when they are equal, because it published that event itself.
	// NOTE: the cacheID of two distributed cache instances is never the same.
	cacheID  string
	hostname string

	// stats
	hits        atomic.Int64
	misses      atomic.Int64
	deletes     atomic.Int64
	peerSets    atomic.Int64
	peerDeletes atomic.Int64

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
	// wmMu serializes the check-then-act on appliedTS so the concurrent
	// writers (the event listener and local Set/Delete callers) cannot
	// regress the watermark.
	wmMu sync.Mutex

	// pub publishes this instance's set and delete events; sub receives the
	// events of every instance, including this one's own, which are skipped
	// by cacheID.
	pub *kgo.Client
	sub *kgo.Client

	// logger is the cache internal logger.
	logger types.Logger

	// gopool bounds the goroutines publishing events.
	gopool *ants.Pool
}

// newDistributedCache creates and initializes one distributed cache instance
// around the given store and a kafka producer/consumer pair. The store is a
// parameter so tests can watch propagation between two instances holding
// separate stores.
func newDistributedCache[T any](store types.Cache[entry[T]]) (*distributedCache[T], error) {
	cacheID := uuid.NewV7()
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	dc := &distributedCache[T]{
		store:    store,
		cacheID:  cacheID.String(),
		typ:      typeName[T](),
		hostname: hostname,
	}
	// comp marks the distributed cache name so its log lines are searchable.
	comp := fmt.Sprintf("[%s:DistributedCache:%s]", hostname, reflect.TypeFor[T]().String())
	dc.logger = logger.Dcache.With("hostname", hostname, compKey, comp)

	// setup kafka; the consumer group name follows the topic
	if dc.appliedTS, err = lru.New[string, int64](maxTrackedKeys); err != nil {
		return nil, err
	}
	dc.topic = cacheTopic()
	brokers := config.App.Kafka.Brokers
	if dc.pub, err = newProducer(brokers, dc.topic); err != nil {
		return nil, err
	}
	if dc.sub, err = newConsumer(brokers, dc.topic, dc.topic); err != nil {
		return nil, err
	}

	// setup the goroutine pool: the heuristic capacity is raised to the
	// floor, which matters on a small machine.
	gocap := max(runtime.NumCPU()*2000, minGoroutines)
	pool, err := ants.NewPool(gocap, ants.WithPreAlloc(false))
	if err != nil {
		return nil, err
	}
	dc.gopool = pool

	dc.listenEvents()
	dc.startMonitor()

	return dc, nil
}

// Set sets a key-value pair in the store and publishes an opSet event that
// carries the value to the store of every other instance.
func (dc *distributedCache[T]) Set(ctx context.Context, key string, value T, ttl time.Duration) (err error) {
	if ttl < 0 {
		return errors.New("negative ttl")
	}

	// Set the own store first and report its failure: publishing the event
	// anyway would leave every other instance holding a value this one does
	// not have, while the caller was told the write succeeded.
	if err = dc.store.Set(ctx, key, entry[T]{Value: value}, ttl); err != nil {
		return err
	}

	// The local write enters the same watermark as accepted peer events, so
	// an older peer event still in flight cannot overwrite it.
	ts := time.Now().UnixNano()
	dc.advanceWatermark(key, ts)
	dc.sendEvent(&event{
		TS:  ts,
		Op:  opSet,
		Key: key,
		raw: value,
		TTL: ttl,
	})

	return nil
}

func (dc *distributedCache[T]) Get(ctx context.Context, key string) (value T, err error) {
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
// removes it from the store of every other instance.
func (dc *distributedCache[T]) Delete(ctx context.Context, key string) (err error) {
	dc.deletes.Add(1)

	if err = dc.store.Delete(ctx, key); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
		return err
	}

	// See Set: the local delete enters the watermark, so a stale peer set
	// cannot resurrect the key this instance just removed.
	ts := time.Now().UnixNano()
	dc.advanceWatermark(key, ts)
	dc.sendEvent(&event{
		TS:  ts,
		Op:  opDel,
		Key: key,
	})

	return nil
}

func (dc *distributedCache[T]) Exists(ctx context.Context, key string) bool {
	return dc.store.Exists(ctx, key)
}

// logPanic recovers a panic that would otherwise kill the process and turns
// it into a structured error record. The goroutine still ends: for the event
// listener that means the instance degrades to a process-local cache
// reconciled by TTLs, and restarting is a lifecycle this package
// deliberately does not have.
func (dc *distributedCache[T]) logPanic(goroutine string) {
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
func (dc *distributedCache[T]) listenEvents() {
	go func() {
		defer func() {
			if dc.gopool != nil {
				dc.gopool.Release()
			}
		}()
		defer dc.logPanic("DistributedCache.listenEvents")

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
				if !dc.shouldApply(evt) {
					continue
				}
				dc.logger.Debugz("consume event", zap.Any("event", evt.logView()))
				switch evt.Op {
				case opSet:
					var val T
					if err := json.Unmarshal(evt.Val, &val); err != nil {
						dc.logger.Errorz("failed to unmarshal event value", zap.Error(err), zap.Any("event", evt.logView()))
						continue
					}
					dc.peerSets.Add(1)
					if err := dc.store.Set(context.Background(), evt.Key, entry[T]{Value: val}, evt.TTL); err != nil {
						dc.logger.Warnz("failed to set to the store", zap.Error(err))
						continue
					}
					dc.advanceWatermark(evt.Key, evt.TS)
				case opDel:
					dc.peerDeletes.Add(1)
					if err := dc.store.Delete(context.Background(), evt.Key); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
						dc.logger.Warnz("failed to delete from the store", zap.Error(err))
						continue
					}
					dc.advanceWatermark(evt.Key, evt.TS)
				default:
					dc.logger.Warnz("unknown event op", zap.Any("event", evt.logView()))
				}
			}
		}
	}()
}

// shouldApply reports whether an event is newer than the watermark of its
// key. Out-of-order delivery is rare but not impossible, and applying a
// stale set after a delete would bring the entry back on every instance
// that saw them in that order. The watermark itself advances only through
// advanceWatermark, after the event took effect, so a failed application
// does not claim its timestamp.
func (dc *distributedCache[T]) shouldApply(evt *event) bool {
	dc.wmMu.Lock()
	last, ok := dc.appliedTS.Get(evt.Key)
	dc.wmMu.Unlock()
	if ok && evt.TS <= last {
		dc.logger.Warnz(
			"skipping out-of-order event",
			zap.String("key", evt.Key),
			zap.Int64("event_ts", evt.TS),
			zap.Int64("applied_ts", last),
			zap.String("op", evt.Op.String()),
		)
		return false
	}
	return true
}

// advanceWatermark records ts as the newest applied timestamp of key when it
// is newer than the recorded one. Local writes and accepted peer events feed
// the same watermark; wmMu keeps the check-then-act monotonic under
// concurrent callers.
func (dc *distributedCache[T]) advanceWatermark(key string, ts int64) {
	dc.wmMu.Lock()
	defer dc.wmMu.Unlock()
	if last, ok := dc.appliedTS.Get(key); ok && ts <= last {
		return
	}
	dc.appliedTS.Add(key, ts)
}

// sendEvent asynchronously publishs cache update or delete events to
// kafka topic using a controlled goroutines pool to prevent excessive
// goroutines creation and properly handle sub-groutines panic.
func (dc *distributedCache[T]) sendEvent(evt *event) {
	if evt == nil {
		return
	}
	// Marshal the caller's value synchronously: doing it in the pool goroutine
	// would keep reading the value concurrently after Set has returned.
	val, err := json.Marshal(evt.raw)
	if err != nil {
		dc.logger.Errorz("failed to marshal event raw data", zap.Error(err), zap.Any("event", evt.logView()))
		return
	}
	if len(val) == 0 {
		dc.logger.Warnz("the marshaled value is empty", zap.Any("event", evt.logView()))
		return
	}
	evt.CacheID = dc.cacheID
	evt.Typ = dc.typ
	evt.Val = val
	evt.Hostname = dc.hostname
	evt.raw = nil // clear it to keep the event small
	data, err := json.Marshal(evt)
	if err != nil {
		dc.logger.Errorz("failed to marshal event", zap.Error(err), zap.Any("event", evt.logView()))
		return
	}
	err = dc.gopool.Submit(func() {
		record := &kgo.Record{
			Topic: dc.topic,
			// The key pins every event for one cache key to one partition.
			// Without it the partitioner spreads them, and Kafka only orders
			// within a partition, so a delete could be applied before an
			// older set that follows it.
			Key:   []byte(evt.Key),
			Value: data,
		}
		dc.logger.Debugz("publish event", zap.Any("event", evt.logView()))
		if pubErr := dc.pub.ProduceSync(context.Background(), record).FirstErr(); pubErr != nil {
			dc.logger.Errorz("failed to publish event", zap.Error(pubErr), zap.Any("event", evt.logView()))
		}
	})
	if err != nil {
		dc.logger.Errorz("failed to submit event to gopool", zap.Error(err))
	}
}

func (dc *distributedCache[T]) startMonitor() {
	ticker := time.NewTicker(3 * time.Minute)
	go func() {
		defer dc.logPanic("DistributedCache.startMonitor")
		for range ticker.C {
			if flag.Lookup("test.v") == nil {
				dc.logger.Infoz("cache metrics", zap.Any("metrics", dc.Metrics()))
			}
		}
	}()
}

func (dc *distributedCache[T]) Metrics() *distributedMetrics {
	return &distributedMetrics{
		Hits:    dc.hits.Load(),
		Misses:  dc.misses.Load(),
		Ratio:   calculateHitRatio(dc.hits.Load(), dc.misses.Load()),
		Deletes: dc.deletes.Load(),

		PeerSets:    dc.peerSets.Load(),
		PeerDeletes: dc.peerDeletes.Load(),

		GoroutinesPoolCap: int64(dc.gopool.Cap()),
		GoroutinesUsed:    int64(dc.gopool.Running()),
	}
}

// distributedMetrics is the monitor snapshot of one instance's counters. The
// peer counters record how many peer events this instance applied to its
// store.
type distributedMetrics struct {
	Hits    int64 `json:"hits"`
	Misses  int64 `json:"misses"`
	Ratio   int64 `json:"ratio"`
	Deletes int64 `json:"deletes"`

	PeerSets    int64 `json:"peer_sets"`
	PeerDeletes int64 `json:"peer_deletes"`

	GoroutinesPoolCap int64 `json:"goroutines_pool_cap"`
	GoroutinesUsed    int64 `json:"goroutines_used"`
}
