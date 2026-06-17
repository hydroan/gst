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

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
	cmap "github.com/orcaman/concurrent-map/v2"
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
	opSetDone
	opDelDone
)

var (
	// 为什么选择 cmap v2
	//  1. sync.Map 不支持泛型, 在大量使用泛型的缓存库里面不使用泛型很突兀/麻烦
	//  2. cmap v2 比 sync.Map 性能要高很多
	distributedCacheMap = cmap.New[any]()
	distributedCacheMu  sync.Mutex

	_ types.DistributedCache[any] = (*distributedCache[any])(nil)
)

type op int

func (o op) String() string {
	switch o {
	case opSet:
		return "set"
	case opDel:
		return "del"
	case opSetDone:
		return "set_done"
	case opDelDone:
		return "del_done"
	default:
		return "unknown"
	}
}

type event struct {
	CacheID string

	Key string // redis key
	TS  int64  // timestamp
	Op  op
	Val json.RawMessage
	Typ string
	raw any
	TTL time.Duration

	Hostname string // 哪台服务器产生的事件

	SyncToRedis bool
	RedisTTL    time.Duration
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
	enc.AddString("ts", time.Unix(0, e.TS).Format("2006-01-02 15:04:05"))
	enc.AddString("cache_id", e.CacheID)
	enc.AddString("hostname", e.Hostname)
	enc.AddString("typ", e.Typ)
	enc.AddString("op", e.Op.String())
	enc.AddString("key", e.Key)
	_ = enc.AddReflected("value", val)
	enc.AddString("local_ttl", util.FormatDurationSmart(e.TTL, 2))
	enc.AddString("redis_ttl", util.FormatDurationSmart(e.RedisTTL, 2))
	enc.AddBool("sync_to_redis", e.SyncToRedis)

	return nil
}

// NewDistributedCache 为什么要为每种类型创建一个单独的缓存, 并放在一个并发 map 中?
// 每个类型的缓存都有自己的 goroutine 来监控 opSetDone, opDelDone 事件, 互不干涉
// 因为数据类型有限, 所以不会有太多的 goroutine 监听 kafka 事件, 监听者不多, 则效率会更高.
//
// 如果不这么做, 每调用一次 NewDistributedCache 就会创建一个 goroutine 监听 kafka 事件.
// 会导致创建过多的 kafka 消费者, 这完全不是我们想要的.
// 既然提供了这个函数, 我们没办法完全保证其他开发者不会频繁调用这个函数, 控制权还需要交给自己.
//
// 计算:
//
//	kafka 在单节点上的消费者数量: 服务进程个数 * DistributedCache个数, 一般都是跑一个服务进程的.
//	kafka 监听者总数量: 但节点上消费者个数 * 节点个数
func NewDistributedCache[T any](opts ...DistributedCacheOption[T]) (types.DistributedCache[T], error) {
	typ := reflect.TypeFor[T]()
	key := typ.PkgPath() + "|" + typ.String()

	// Fast path: check if cache already exists
	val, exists := distributedCacheMap.Get(key)
	if exists {
		return val.(*distributedCache[T]), nil //nolint:errcheck
	}

	distributedCacheMu.Lock()
	defer distributedCacheMu.Unlock()

	// Double-check after acquiring lock
	val, exists = distributedCacheMap.Get(key)
	if !exists {
		cache, err := newDistributedCache(opts...)
		if err != nil {
			return nil, err
		}
		val = cache
		distributedCacheMap.Set(key, cache)
	}
	return val.(*distributedCache[T]), nil //nolint:errcheck
}

// distributedCache implements a two-level cacheing system with local memery cache and redis backend.
// It provides cache synchronization across multiple instances using kafka for event publishing and consuming:
//   - Local memory cache for high-speed access.
//   - Redis for distributed persistence and high availability.
//   - Kafka for cross-instance cache invalidation.
//
// Performance metrics are tracked (hits/misses) and a controlled goroutine pool handles
type distributedCache[T any] struct {
	localCache types.Cache[T]
	redisCache types.Cache[any]

	// 用来在 redis 缓存中区分不同的类型
	prefix string

	// typ 分布式缓存类型
	// 当某一个实例收到 opSetDone, opDelDone 事件时, 会检查 event.Typ 是否于等于自己的分布式缓存类型
	// 如果相同, 则不处理.
	// NOTE: 多个分布式缓存实例的 typ 总是相同
	typ string

	// 分布式缓存ID, 用来标识不同的分布式缓存实例
	// 每一个实例都有自己唯一的分布式缓存ID
	// 当某一个实例收到 opSetDone, opDelDone 事件时, 会检查 event.CacheId 是否于等于自己的分布式缓存ID
	// 如果相同, 则不处理.
	// NOTE: 多个分布式缓存实例的 cacheID 总是不同
	cacheID  string
	hostname string

	// stats
	localHits         atomic.Int64
	localMisses       atomic.Int64
	localDelete       atomic.Int64
	redisHits         atomic.Int64
	redisMisses       atomic.Int64
	distributedSet    atomic.Int64
	distributedDelete atomic.Int64

	kafkaBrokers []string
	// pubSetDel is the kafka producer, publish the event that the entry associated with the key was updated/delete.
	pubSetDel *kgo.Client
	// subDone is the kafka consumer, receive the event that the entry associated with the key was updated/delete.
	subDone *kgo.Client

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

// newDistributedCache creates and initializes a new Distributed Cache system with local and Redis backend.
// Parameters:
//   - localCache: In-Memory cache implementation for fast access.
//   - brokers: kafka brokers addresses for event publishing and consuming.
//   - opts: Optional configuration options.
func newDistributedCache[T any](opts ...DistributedCacheOption[T]) (types.Cache[T], error) {
	cacheID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	// 为什么这里要加上 prefix?
	// localCache 是支持泛型的, 每一种类型都有单独的 localCache,
	// NewLocalCache 只是从包含多个 local cache 的 map 中返回当前类型的 lcoal cache
	// 由于 redis 是不支持泛型的, 所以这里加上一个 prefix 来作为新的命名空间
	typ := reflect.TypeFor[T]()
	var prefix string
	var typStr string
	if len(typ.PkgPath()) > 0 { // 不是 golang 基本类型, 一般是结构体类型
		prefix = fmt.Sprintf("%s:%s:", typ.PkgPath(), typ.Name())
		typStr = fmt.Sprintf("%s:%s", typ.PkgPath(), typ.Name())
	} else { // golang 基本类型
		prefix = typ.Name() + ":"
		typStr = typ.Name()
	}

	dc := &distributedCache[T]{
		cacheID:  cacheID.String(),
		prefix:   prefix,
		typ:      typStr,
		comp:     fmt.Sprintf("[%s:DistributedCache:%s]", hostname, typ.Name()),
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

	// setup redis cache
	if dc.redisCache == nil {
		redisCli, e := redis.New(config.App.Redis)
		if err != nil {
			return nil, e
		}
		if dc.redisCache, e = NewRedisCache[any](context.Background(), redisCli); e != nil {
			return nil, e
		}
		if dc.redisCache == nil {
			return nil, errors.New("redis cache is nil")
		}
	}

	// setup kafka
	if len(dc.kafkaBrokers) == 0 {
		dc.kafkaBrokers = config.App.Kafka.Brokers
	}
	if dc.pubSetDel, err = newProducer(dc.kafkaBrokers, TOPIC_REDIS_SET_DEL); err != nil {
		return nil, err
	}
	if dc.subDone, err = newConsumer(dc.kafkaBrokers, TOPIC_REDIS_DONE, GROUP_REDIS_DONE); err != nil {
		return nil, err
	}

	// setup goroutines pool.
	if dc.gocap < MIN_GOROUTINES {
		dc.gocap = runtime.NumCPU() * 2000
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

// Set sets a key-value pair in the local cache and publishs an event "OpSet"
// to invalidate redis cache.
func (dc *distributedCache[T]) Set(key string, value T, ttl time.Duration) (err error) {
	// done := dc.trace("Set")
	// defer done(err)

	prefixedKey := dc.prefix + key

	// set local cache.
	if err = dc.localCache.Set(prefixedKey, value, ttl); err != nil {
		dc.logger.Warn("failed to set local cache", zap.Error(err))
	}

	dc.sendEvent(&event{
		TS:  time.Now().UnixNano(),
		Op:  opSet,
		Key: prefixedKey,
		raw: value,
		TTL: ttl,
	})

	return nil
}

func (dc *distributedCache[T]) SetWithSync(key string, value T, localTTL time.Duration, remoteTTL time.Duration) (err error) {
	// done := dc.trace("Set")
	// defer done(err)

	if remoteTTL < localTTL {
		return errors.New("remoteTTL must be greater than localTTL")
	}
	prefixedKey := dc.prefix + key

	// set local cache.
	if err = dc.localCache.Set(prefixedKey, value, localTTL); err != nil {
		dc.logger.Warn("failed to set local cache", zap.Error(err))
	}

	dc.sendEvent(&event{
		TS:  time.Now().UnixNano(),
		Op:  opSet,
		Key: prefixedKey,
		raw: value,
		TTL: localTTL,

		SyncToRedis: true,
		RedisTTL:    remoteTTL,
	})

	return nil
}

func (dc *distributedCache[T]) Get(key string) (value T, err error) {
	// done := dc.trace("Get")
	// defer done(err)

	prefixedKey := dc.prefix + key

	// get from local cache.
	if value, err = dc.localCache.Get(prefixedKey); err == nil {
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

	dc.logger.Warn("failed to get from local cache", zap.Error(err))
	return zero, err
}

func (dc *distributedCache[T]) GetWithSync(key string, localTTL time.Duration) (value T, err error) {
	// done := dc.trace("Get")
	// defer done(err)

	prefixedKey := dc.prefix + key

	var zero T
	// get from local cache.
	if value, err = dc.localCache.Get(prefixedKey); err == nil {
		// local cache hit.
		dc.localHits.Add(1)
		return value, nil
	}
	if errors.Is(err, types.ErrEntryNotFound) {
		// local cache miss.
		dc.localMisses.Add(1)
	} else {
		dc.logger.Warn("failed to get from local cache", zap.Error(err))
		return zero, err
	}

	var (
		redisVal T
		result   any
		ok       bool
	)
	// get from redis cache
	if result, err = dc.redisCache.Get(prefixedKey); err == nil {
		if redisVal, ok = result.(T); !ok {
			dc.logger.Warn(fmt.Sprintf("type assertion failed for key %s: expected %T, got %T", prefixedKey, *new(T), result))
			return zero, types.ErrEntryNotFound
		}
		// redis cache hit.
		dc.redisHits.Add(1)
		if err = dc.localCache.Set(prefixedKey, redisVal, localTTL); err != nil {
			dc.logger.Warn("failed to set local cache", zap.Error(err))
			return redisVal, err
		}
		return redisVal, nil
	}
	if errors.Is(err, types.ErrEntryNotFound) {
		// redis cache miss.
		dc.redisMisses.Add(1)
		return zero, types.ErrEntryNotFound
	}
	dc.logger.Warn("failed to get from redis cache", zap.Error(err))
	return zero, err
}

func (dc *distributedCache[T]) Delete(key string) (err error) {
	// done := dc.trace("Delete")
	// defer done(err)

	dc.localDelete.Add(1)
	prefixedKey := dc.prefix + key

	// NOTE: After recive kafka "delete" event, we will delete the entry from local cache again, it is a no-op.
	if err = dc.localCache.Delete(prefixedKey); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
		dc.logger.Warn("failed to delete from local cache", zap.Error(err))
	}

	dc.sendEvent(&event{
		TS:  time.Now().UnixNano(),
		Op:  opDel,
		Key: prefixedKey,
	})

	return nil
}

func (dc *distributedCache[T]) DeleteWithSync(key string) (err error) {
	// done := dc.trace("Delete")
	// defer done(err)

	dc.localDelete.Add(1)
	prefixedKey := dc.prefix + key

	// NOTE: After recive kafka "delete" event, we will delete the entry from local cache again, it is a no-op.
	if err = dc.localCache.Delete(prefixedKey); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
		dc.logger.Warn("failed to delete from local cache", zap.Error(err))
	}

	dc.sendEvent(&event{
		TS:  time.Now().UnixNano(),
		Op:  opDel,
		Key: prefixedKey,

		SyncToRedis: true,
	})

	return nil
}

func (dc *distributedCache[T]) Exists(key string) bool {
	return dc.localCache.Exists(dc.prefix + key)
}
func (dc *distributedCache[T]) Len() int                                   { return -1 }
func (dc *distributedCache[T]) Peek(string) (T, error)                     { var t T; return t, nil }
func (dc *distributedCache[T]) Clear()                                     {}
func (dc *distributedCache[T]) WithContext(context.Context) types.Cache[T] { return dc }

// listenEvents listen kafka for cache update/delete event and synchronously update the local cache.
func (dc *distributedCache[T]) listenEvents() {
	records := make([]*kgo.Record, 0, 1024)

	util.SafeGo(func() {
		defer func() {
			if dc.gopool != nil {
				dc.gopool.Release()
			}
		}()

		for {
			fetches := dc.subDone.PollFetches(context.Background())
			if fetches.IsClientClosed() {
				// TODO: reconnect
				continue
			}
			fetches.EachError(func(s string, i int32, err error) {
				dc.logger.Error(
					"failed to fetch from kafka",
					zap.Error(err),
					zap.String("topic", TOPIC_REDIS_DONE),
					zap.String("s", s),
					zap.Int32("i", i),
				)
			})
			records = fetches.Records()
			if len(records) == 0 {
				continue
			}
			for _, record := range records {
				evt := new(event)
				if err := json.Unmarshal(record.Value, evt); err != nil {
					dc.logger.Error(
						"failed to unmarshal event",
						zap.Error(err),
						zap.String("topic", TOPIC_REDIS_DONE),
						zap.ByteString("value", record.Value),
					)
					continue
				}
				switch evt.Op {
				case opSetDone:
					// 如果是自己发出的事件，跳过处理
					// 先检查缓存ID, 检查完后其实不用再检查缓存类型
					if evt.CacheID == dc.cacheID {
						// fmt.Println("----- set 缓存ID不匹配", dc.mark, dc.cacheId, evt.CacheId)
						continue
					}
					// 这里会接收到任意类型的数据, 基本类型,自定义类型等, 需要判断是否是自己的类型
					// 不用担心不同类型会有相同的key而导致错误的设置,不同类型的key总是会不同的, 例如:
					// key1 在 string 类型的 localCache, redisCache 是这样的: string:key1
					// key1 在 int 类型的 localCache, redisCache 是这样的: int:key1
					if evt.Typ != dc.typ {
						// fmt.Println("----- set 缓存类型不匹配", dc.mark, dc.typ, evt.Typ)
						continue
					}

					// TODO: 生产环境需要设置成 debug
					dc.logger.Info("consume event", zap.Object("event", evt))
					var val T
					// fmt.Printf("----- %s OpSet %v %v %v\n", dc.mark, event.Typ, event.Key, string(event.Val))
					if err := json.Unmarshal(evt.Val, &val); err == nil {
						// TODO: 如何解决这个问题
						// 本地缓存已经删除了, 收到 opSetDone 事件后,又要再删除一次, 我觉得没必要重复删除

						dc.distributedSet.Add(1)
						// 这里不需要使用 prefix + key, 状态节点传过来的 key, 已经是 prefix+key 了.
						if err := dc.localCache.Set(evt.Key, val, evt.TTL); err != nil {
							dc.logger.Warn("failed to set to local cache", zap.Error(err))
						}
					}
				case opDelDone:
					// 先检查缓存ID, 其实不用再检查缓存类型
					if evt.CacheID == dc.cacheID {
						// fmt.Println("------ delete 缓存ID不匹配", dc.mark, dc.cacheId, evt.CacheId)
						continue
					}
					if evt.Typ != dc.typ {
						// fmt.Println("------ delete 缓存类型不匹配:", dc.mark, dc.typ, evt.Typ)
						continue
					}
					dc.distributedDelete.Add(1)
					// 这里不需要使用 prefix + key, 状态节点传过来的 key, 已经是 prefix+key 了.
					// 但凡收到 opDelDone 事件, 都需要从本地缓存中删除, 我们无法得知这个 key 是不是属于我们当前缓存的
					if err := dc.localCache.Delete(evt.Key); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
						dc.logger.Warn("failed to delete from local cache", zap.Error(err))
					}
				default:
					dc.logger.Warn("unknown event op", zap.String("op", evt.Op.String()), zap.String("key", evt.Key), zap.Object("event", evt))
				}
			}

			// reset slice and keep the underlying array.
			records = records[:0]
		}
	}, "DistributedCache.listenEvents")
}

// sendEvent asynchronously publishs cache update or delete events to
// kafka topic using a controlled goroutines pool to prevent excessive
// goroutines creation and properly handle sub-groutines panic.
func (dc *distributedCache[T]) sendEvent(evt *event) {
	if evt == nil {
		return
	}
	err := dc.gopool.Submit(func() {
		val, err := json.Marshal(evt.raw)
		if err != nil {
			dc.logger.Error("failed to marshal event raw data", zap.Error(err), zap.Object("event", evt))
			return
		}
		if len(val) == 0 {
			dc.logger.Warn("the marshaled value is empty", zap.Object("event", evt))
			return
		}
		evt.CacheID = dc.cacheID
		evt.Typ = dc.typ
		evt.Val = val
		evt.Hostname = dc.hostname
		evt.raw = nil // 设置为nil,减少event体积
		data, err := json.Marshal(evt)
		if err != nil {
			dc.logger.Error("failed to marshal event", zap.Error(err), zap.Object("event", evt))
			return
		}
		record := &kgo.Record{
			Topic: TOPIC_REDIS_SET_DEL,
			Value: data,
		}
		// TODO: 日志设置成 debug
		dc.logger.Info("publish event", zap.Object("event", evt))
		if err := dc.pubSetDel.ProduceSync(context.Background(), record).FirstErr(); err != nil {
			dc.logger.Error("failed to publish event", zap.Error(err), zap.Object("event", evt))
		}
	})
	if err != nil {
		dc.logger.Error("failed to submit event to gopool", zap.Error(err))
	}
}

func (dc *distributedCache[T]) startMonitor() {
	ticker := time.NewTicker(3 * time.Minute)
	util.SafeGo(func() {
		for range ticker.C {
			if flag.Lookup("test.v") == nil {
				if local, ok := dc.localCache.(CacheMetricsProvider); ok {
					dc.logger.Info("cache metrics", zap.Object("distributed", dc.Metrics()), zap.Object("local", local.Metrics()))
				} else {
					dc.logger.Info("cache metrics", zap.Object("distributed", dc.Metrics()))
				}
			}
		}
	}, "DistributedCache.startMonitor")
}

func (dc *distributedCache[T]) Metrics() *distributedMetrics {
	return &distributedMetrics{
		LocalHists:  dc.localHits.Load(),
		LocalMisses: dc.localMisses.Load(),
		LocalRatio:  calculateHitRatio(dc.localHits.Load(), dc.localMisses.Load()),
		LocalDelete: dc.localDelete.Load(),

		RedisHits:   dc.redisHits.Load(),
		RedisMisses: dc.redisMisses.Load(),

		DistributedSet:    dc.distributedSet.Load(),
		DistributedDelete: dc.distributedDelete.Load(),

		GoroutinesPoolCap: int64(dc.gopool.Cap()),
		GoroutinesUsed:    int64(dc.gopool.Running()),
	}
}

type distributedMetrics struct {
	LocalHists  int64
	LocalMisses int64
	LocalRatio  int64
	LocalDelete int64

	RedisHits   int64
	RedisMisses int64

	DistributedSet    int64
	DistributedDelete int64

	GoroutinesPoolCap int64
	GoroutinesUsed    int64
}

func (m *distributedMetrics) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if m == nil {
		return nil
	}

	enc.AddInt64("local_hits", m.LocalHists)
	enc.AddInt64("local_misses", m.LocalMisses)
	enc.AddInt64("local_ratio", m.LocalRatio)
	enc.AddInt64("local_delete", m.LocalDelete)
	enc.AddInt64("redis_hists", m.RedisHits)
	enc.AddInt64("redis_misses", m.RedisMisses)
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
			dc.logger.Error("trace", zap.Error(err), zap.String("op", op), zap.String("cost", util.FormatDurationSmart(time.Since(begin), 2)))
		} else {
			dc.logger.Info("trace", zap.String("op", op), zap.String("cost", util.FormatDurationSmart(time.Since(begin), 2)))
		}
	}
}

type DistributedCacheOption[T any] func(*distributedCache[T]) error

func WithRedisCache[T any](redisCache types.Cache[any]) DistributedCacheOption[T] {
	return func(dc *distributedCache[T]) error {
		dc.redisCache = redisCache
		return nil
	}
}

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
