package dcache

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"uuid"

	"github.com/cockroachdb/errors"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"github.com/panjf2000/ants/v2"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	compKey = "comp"
)

const (
	opSet op = iota
	opDel
)

var (
	// One instance per value type, keyed by the type itself. Entries are
	// written once, when a type is first cached, and read for the rest of
	// the process, which is what sync.Map's lock-free read path is for.
	distributedCacheMap sync.Map
	distributedCacheMu  sync.Mutex

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

func (e *event) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if e == nil {
		return nil
	}

	var val []byte
	if len(e.Val) > 1024 {
		val = e.Val[:1024]
	} else {
		val = e.Val
	}
	enc.AddString("ts", time.Unix(0, e.TS).UTC().Format(consts.LayoutTimeEncoder))
	enc.AddString("cache_id", e.CacheID)
	enc.AddString("hostname", e.Hostname)
	enc.AddString("typ", e.Typ)
	enc.AddString("op", e.Op.String())
	enc.AddString("key", e.Key)
	// AddByteString keeps the payload readable. Assigning a json.RawMessage
	// to a []byte drops its Marshaler, so AddReflected would base64 it and
	// the truncation above would lose its purpose.
	enc.AddByteString("value", val)
	enc.AddString("ttl", util.FormatDurationSmart(e.TTL, 2))

	return nil
}

// NewDistributedCache returns the distributed cache of type T, creating it on first use.
//
// Why keep one cache per type in a concurrent map?
// Each type's cache owns the goroutine that watches the peer set and delete events, and they
// never interfere with each other. The number of types is bounded, so only a few goroutines ever
// listen for kafka events, and fewer listeners means better throughput.
//
// Without the map, every NewDistributedCache call would spawn another goroutine listening for
// kafka events and therefore far too many kafka consumers, which is not what we want. Since this
// function is exported we cannot stop other developers from calling it repeatedly, so the control
// has to stay here.
//
// The arithmetic:
//
//	kafka consumers on one node: number of service processes * number of DistributedCache instances,
//	and normally only one service process runs.
//	total kafka listeners: consumers per node * number of nodes
func NewDistributedCache[T any](opts ...DistributedCacheOption[T]) (types.Cache[T], error) {
	key := reflect.TypeFor[T]()

	// Fast path: check if cache already exists. Options only take effect when
	// the instance is created, so a later caller passing its own would
	// silently get someone else's configuration.
	if val, ok := distributedCacheMap.Load(key); ok {
		if len(opts) > 0 {
			return nil, errors.Newf("distributed cache for %s already exists; options only apply to the first call", key)
		}
		return val.(*distributedCache[T]), nil //nolint:errcheck
	}

	distributedCacheMu.Lock()
	defer distributedCacheMu.Unlock()

	// Double-check after acquiring lock
	if val, ok := distributedCacheMap.Load(key); ok {
		return val.(*distributedCache[T]), nil //nolint:errcheck
	}
	cache, err := newDistributedCache(opts...)
	if err != nil {
		return nil, err
	}
	distributedCacheMap.Store(key, cache)
	return cache, nil
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
	localCache types.Cache[T]

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
	localHits         atomic.Int64
	localMisses       atomic.Int64
	localDelete       atomic.Int64
	distributedSet    atomic.Int64
	distributedDelete atomic.Int64

	kafkaBrokers []string
	// topic carries the set and delete events of every instance; it is
	// resolved once at construction and the default derives from the
	// application name.
	topic string
	// appliedTS is the highest event timestamp already applied per key. The
	// partition key keeps one key's events ordered within a partition, but a
	// partition count change or a replay can still deliver an older event
	// afterwards, and applying it would resurrect a deleted entry. Bounded
	// by LRU: evicting a key only reopens that key's out-of-order window.
	appliedTS *lru.Cache[string, int64]

	// pub publishes this instance's set and delete events; sub receives the
	// events of every instance, including this one's own, which are skipped
	// by cacheID.
	pub *kgo.Client
	sub *kgo.Client

	// logger is the cache internal logger, call "WithLogger" to replace it.
	logger types.Logger

	// "gopool" is the goroutines pool, the pool capacity is determined by "gocap".
	// call "WithMaxGoroutines" to set the goroutines pool capacity.
	gocap  int
	gopool *ants.Pool

	// call "WithTrace" to enable set traceEnabled to true to logger each operation costed time.
	traceEnabled bool
	// comp is used to mark the distributed cache name that is convenient for logger search.
	comp string
}

// newDistributedCache creates and initializes one distributed cache instance
// with its local tier and kafka producer/consumer pair.
func newDistributedCache[T any](opts ...DistributedCacheOption[T]) (*distributedCache[T], error) {
	cacheID := uuid.NewV7()
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	dc := &distributedCache[T]{
		cacheID:  cacheID.String(),
		typ:      typeName[T](),
		comp:     fmt.Sprintf("[%s:DistributedCache:%s]", hostname, reflect.TypeFor[T]().String()),
		hostname: hostname,
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err = opt(dc); err != nil {
			return nil, err
		}
	}

	// setup logger
	if dc.logger == nil {
		dc.logger = logger.Dcache.With("hostname", hostname, compKey, dc.comp)
	}

	// setup local cache.
	if dc.localCache == nil {
		if dc.localCache, err = NewLocalCache[T](); err != nil {
			return nil, err
		}
		if dc.localCache == nil {
			return nil, errors.New("local cache is nil")
		}
	}

	// setup kafka; the consumer group name follows the topic
	if len(dc.kafkaBrokers) == 0 {
		dc.kafkaBrokers = config.App.Kafka.Brokers
	}
	if dc.appliedTS, err = lru.New[string, int64](maxTrackedKeys); err != nil {
		return nil, err
	}
	dc.topic = cacheTopic()
	if dc.pub, err = newProducer(dc.kafkaBrokers, dc.topic); err != nil {
		return nil, err
	}
	if dc.sub, err = newConsumer(dc.kafkaBrokers, dc.topic, dc.topic); err != nil {
		return nil, err
	}

	// setup goroutines pool: an unset capacity gets the heuristic default,
	// and any capacity is then raised to the floor. Doing it in one step
	// would replace a caller's deliberate 5000 with a value that can be
	// smaller still on a small machine.
	if dc.gocap <= 0 {
		dc.gocap = runtime.NumCPU() * 2000
	}
	if dc.gocap < minGoroutines {
		dc.gocap = minGoroutines
	}
	pool, err := ants.NewPool(dc.gocap, ants.WithPreAlloc(false))
	if err != nil {
		return nil, err
	}
	dc.gopool = pool

	dc.listenEvents()
	dc.startMonitor()

	return dc, nil
}

// Set sets a key-value pair in the local cache and publishes an opSet event
// that carries the value to the local tier of every other instance.
func (dc *distributedCache[T]) Set(ctx context.Context, key string, value T, ttl time.Duration) (err error) {
	// done := dc.trace("Set")
	// defer done(err)

	if ttl < 0 {
		return errors.New("negative ttl")
	}

	// Set the local tier first and report its failure: publishing the event
	// anyway would leave every other instance holding a value this one does
	// not have, while the caller was told the write succeeded.
	if err = dc.localCache.Set(ctx, key, value, ttl); err != nil {
		return err
	}

	dc.sendEvent(&event{
		TS:  time.Now().UnixNano(),
		Op:  opSet,
		Key: key,
		raw: value,
		TTL: ttl,
	})

	return nil
}

func (dc *distributedCache[T]) Get(ctx context.Context, key string) (value T, err error) {
	// done := dc.trace("Get")
	// defer done(err)

	// get from local cache.
	if value, err = dc.localCache.Get(ctx, key); err == nil {
		// local cache hit.
		dc.localHits.Add(1)
		return value, nil
	}
	var zero T
	if errors.Is(err, types.ErrEntryNotFound) {
		// local cache miss.
		dc.localMisses.Add(1)
		return zero, types.ErrEntryNotFound
	}

	dc.logger.Warnz("failed to get from local cache", zap.Error(err))
	return zero, err
}

// Delete removes the entry from the local cache and publishes an opDel event
// that removes it from the local tier of every other instance.
func (dc *distributedCache[T]) Delete(ctx context.Context, key string) (err error) {
	// done := dc.trace("Delete")
	// defer done(err)

	dc.localDelete.Add(1)

	if err = dc.localCache.Delete(ctx, key); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
		return err
	}

	dc.sendEvent(&event{
		TS:  time.Now().UnixNano(),
		Op:  opDel,
		Key: key,
	})

	return nil
}

func (dc *distributedCache[T]) Exists(ctx context.Context, key string) bool {
	return dc.localCache.Exists(ctx, key)
}

// listenEvents consumes the set and delete events published by the other
// instances and applies them to the local tier.
func (dc *distributedCache[T]) listenEvents() {
	util.SafeGo(func() {
		defer func() {
			if dc.gopool != nil {
				dc.gopool.Release()
			}
		}()

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
				// The filter must also run before acceptEvent: appliedTS is
				// keyed by the bare cache key, and recording a foreign type's
				// event would advance the watermark of this type's same-named
				// key.
				if evt.Typ != dc.typ {
					continue
				}
				if !dc.acceptEvent(evt) {
					continue
				}
				dc.logger.Debugz("consume event", zap.Object("event", evt))
				switch evt.Op {
				case opSet:
					var val T
					if err := json.Unmarshal(evt.Val, &val); err != nil {
						dc.logger.Errorz("failed to unmarshal event value", zap.Error(err), zap.Object("event", evt))
						continue
					}
					dc.distributedSet.Add(1)
					if err := dc.localCache.Set(context.Background(), evt.Key, val, evt.TTL); err != nil {
						dc.logger.Warnz("failed to set to local cache", zap.Error(err))
					}
				case opDel:
					dc.distributedDelete.Add(1)
					if err := dc.localCache.Delete(context.Background(), evt.Key); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
						dc.logger.Warnz("failed to delete from local cache", zap.Error(err))
					}
				default:
					dc.logger.Warnz("unknown event op", zap.String("op", evt.Op.String()), zap.String("key", evt.Key), zap.Object("event", evt))
				}
			}
		}
	}, "DistributedCache.listenEvents")
}

// acceptEvent reports whether an event is newer than the last one applied
// for its key, recording it when it is. Out-of-order delivery is rare but not
// impossible, and applying a stale set after a delete would bring the entry
// back on every instance that saw them in that order.
func (dc *distributedCache[T]) acceptEvent(evt *event) bool {
	if last, ok := dc.appliedTS.Get(evt.Key); ok && evt.TS <= last {
		dc.logger.Warnz(
			"skipping out-of-order event",
			zap.String("key", evt.Key),
			zap.Int64("event_ts", evt.TS),
			zap.Int64("applied_ts", last),
			zap.String("op", evt.Op.String()),
		)
		return false
	}
	dc.appliedTS.Add(evt.Key, evt.TS)
	return true
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
		dc.logger.Errorz("failed to marshal event raw data", zap.Error(err), zap.Object("event", evt))
		return
	}
	if len(val) == 0 {
		dc.logger.Warnz("the marshaled value is empty", zap.Object("event", evt))
		return
	}
	evt.CacheID = dc.cacheID
	evt.Typ = dc.typ
	evt.Val = val
	evt.Hostname = dc.hostname
	evt.raw = nil // clear it to keep the event small
	data, err := json.Marshal(evt)
	if err != nil {
		dc.logger.Errorz("failed to marshal event", zap.Error(err), zap.Object("event", evt))
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
		// TODO: lower this log to debug
		dc.logger.Infoz("publish event", zap.Object("event", evt))
		if pubErr := dc.pub.ProduceSync(context.Background(), record).FirstErr(); pubErr != nil {
			dc.logger.Errorz("failed to publish event", zap.Error(pubErr), zap.Object("event", evt))
		}
	})
	if err != nil {
		dc.logger.Errorz("failed to submit event to gopool", zap.Error(err))
	}
}

func (dc *distributedCache[T]) startMonitor() {
	ticker := time.NewTicker(3 * time.Minute)
	util.SafeGo(func() {
		for range ticker.C {
			if flag.Lookup("test.v") == nil {
				if local, ok := dc.localCache.(CacheMetricsProvider); ok {
					dc.logger.Infoz("cache metrics", zap.Object("distributed", dc.Metrics()), zap.Object("local", local.Metrics()))
				} else {
					dc.logger.Infoz("cache metrics", zap.Object("distributed", dc.Metrics()))
				}
			}
		}
	}, "DistributedCache.startMonitor")
}

func (dc *distributedCache[T]) Metrics() *distributedMetrics {
	return &distributedMetrics{
		LocalHits:   dc.localHits.Load(),
		LocalMisses: dc.localMisses.Load(),
		LocalRatio:  calculateHitRatio(dc.localHits.Load(), dc.localMisses.Load()),
		LocalDelete: dc.localDelete.Load(),

		DistributedSet:    dc.distributedSet.Load(),
		DistributedDelete: dc.distributedDelete.Load(),

		GoroutinesPoolCap: int64(dc.gopool.Cap()),
		GoroutinesUsed:    int64(dc.gopool.Running()),
	}
}

type distributedMetrics struct {
	LocalHits   int64
	LocalMisses int64
	LocalRatio  int64
	LocalDelete int64

	DistributedSet    int64
	DistributedDelete int64

	GoroutinesPoolCap int64
	GoroutinesUsed    int64
}

func (m *distributedMetrics) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if m == nil {
		return nil
	}

	enc.AddInt64("local_hits", m.LocalHits)
	enc.AddInt64("local_misses", m.LocalMisses)
	enc.AddInt64("local_ratio", m.LocalRatio)
	enc.AddInt64("local_delete", m.LocalDelete)
	enc.AddInt64("distributed_set", m.DistributedSet)
	enc.AddInt64("distributed_delete", m.DistributedDelete)
	enc.AddInt64("goroutines_pool_cap", m.GoroutinesPoolCap)
	enc.AddInt64("goroutines_used", m.GoroutinesUsed)

	return nil
}

// trace
//
//nolint:unused
func (dc *distributedCache[T]) trace(op string) func(error) {
	if !dc.traceEnabled {
		return func(error) {}
	}

	begin := time.Now()
	return func(err error) {
		if err != nil {
			dc.logger.Errorz("trace", zap.Error(err), zap.String("op", op), util.LogDuration(time.Since(begin)))
		} else {
			dc.logger.Infoz("trace", zap.String("op", op), util.LogDuration(time.Since(begin)))
		}
	}
}

type DistributedCacheOption[T any] func(*distributedCache[T]) error

func WithLocalCache[T any](localCache types.Cache[T]) DistributedCacheOption[T] {
	return func(dc *distributedCache[T]) error {
		dc.localCache = localCache
		return nil
	}
}

func WithLogger[T any](logger types.Logger) DistributedCacheOption[T] {
	return func(dc *distributedCache[T]) error {
		if logger == nil {
			return errors.New("logger is nil")
		}
		dc.logger = logger
		return nil
	}
}

func WithMaxGoroutines[T any](maxGoRoutines int) DistributedCacheOption[T] {
	return func(dc *distributedCache[T]) error {
		dc.gocap = maxGoRoutines
		return nil
	}
}

func WithTrace[T any](trace bool) DistributedCacheOption[T] {
	return func(dc *distributedCache[T]) error {
		dc.traceEnabled = trace
		return nil
	}
}

func WithKafkaBrokers[T any](brokers []string) DistributedCacheOption[T] {
	return func(dc *distributedCache[T]) error {
		dc.kafkaBrokers = brokers
		return nil
	}
}
