package redis

// Use the go-redis v8 client with Redis 6 and older,
// and the v9 client with Redis 7 and newer.

import (
	"context"
	"crypto/tls"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
	jsoniter "github.com/json-iterator/go"
	redisotel "github.com/redis/go-redis/extra/redisotel/v9"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

var (
	// cli is the one client handle this package holds. Standalone and cluster
	// connections both satisfy it, so every operation reads the same variable
	// under the same lock: a handle that is absent — never initialized, or
	// closed during shutdown — is absent for all of them at once.
	cli goredis.UniversalClient
	mu  sync.RWMutex

	ErrKeyNotExists    = errors.New("key no longer exists, may be expired")
	ErrRedisIsDisabled = errors.New("redis is disabled")
)

// The two values Redis answers TTL with when there is no deadline to report.
// Both are negative, so a caller that compares the result against a positive
// duration has to test for them before reading it as a remaining lifetime.
//
// They are status codes rather than lengths of time: the client hands the
// reply integer back as a Duration without scaling it by the reply's unit, so
// -2 arrives as -2 nanoseconds. Spelling them in seconds would build
// constants no reply can ever equal, and every comparison against them would
// silently report "not this case".
const (
	// TTLKeyNotExists is reported for a key that does not exist.
	TTLKeyNotExists = time.Duration(-2)
	// TTLNoExpiry is reported for a key that exists without a ttl.
	TTLNoExpiry = time.Duration(-1)
)

func redisKey(key string) string {
	namespace := strings.Trim(config.App.Redis.Namespace, ": ")
	if namespace == "" || hasNamespace(key, namespace) {
		return key
	}
	return namespace + ":" + key
}

// hasNamespace reports whether key already starts with namespace followed by
// the separator. It compares in place because building the prefix to hand to
// strings.HasPrefix would allocate on every key, on every Redis operation.
func hasNamespace(key, namespace string) bool {
	return len(key) > len(namespace) && key[len(namespace)] == ':' && key[:len(namespace)] == namespace
}

func redisKeys(keys []string) []string {
	if len(keys) == 0 {
		return keys
	}
	result := make([]string, len(keys))
	for i := range keys {
		result[i] = redisKey(keys[i])
	}
	return result
}

func redisPattern(prefix string) string {
	if !strings.HasSuffix(prefix, "*") {
		prefix += "*"
	}
	return redisKey(prefix)
}

// sonic library is about 2 times faster than standard library encoding/json.
// var json = sonic.ConfigStd

func Init() (err error) {
	cfg := config.App.Redis
	if !cfg.Enabled {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if cli != nil {
		return nil
	}

	if cfg.ClusterMode {
		var cluster *goredis.ClusterClient
		if cluster, err = NewCluster(cfg); err != nil {
			return errors.Wrap(err, "failed to connect to redis")
		}
		cli = cluster
		zap.S().Infow("successfully connect to redis", "addrs", cfg.Addrs, "cluster_mode", cfg.ClusterMode)
	} else {
		var client *goredis.Client
		if client, err = New(cfg); err != nil {
			return errors.Wrap(err, "failed to connect to redis")
		}
		cli = client
		zap.S().Infow("successfully connect to redis", "addr", cfg.Addr, "db", cfg.DB, "cluster_mode", cfg.ClusterMode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = cli.Ping(ctx).Err(); err != nil {
		cli.Close()
		cli = nil
		return errors.Wrap(err, "failed to ping redis")
	}
	if err = errors.Join(redisotel.InstrumentTracing(cli), redisotel.InstrumentMetrics(cli)); err != nil {
		cli.Close()
		cli = nil
		return err
	}

	return nil
}

func New(cfg config.Redis) (*goredis.Client, error) {
	opts := &goredis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	if cfg.PoolSize > 0 {
		opts.PoolSize = cfg.PoolSize
	}
	if cfg.DialTimeout > 0 {
		opts.DialTimeout = cfg.DialTimeout
	}
	if cfg.ReadTimeout > 0 {
		opts.ReadTimeout = cfg.ReadTimeout
	}
	if cfg.WriteTimeout > 0 {
		opts.WriteTimeout = cfg.WriteTimeout
	}
	if cfg.MinIdleConns > 0 {
		opts.MinIdleConns = cfg.MinIdleConns
	}
	if cfg.MaxRetries > 0 {
		opts.MaxRetries = cfg.MaxRetries
	}
	if cfg.MinRetryBackoff > 0 {
		opts.MinRetryBackoff = cfg.MinRetryBackoff
	}
	if cfg.MaxRetryBackoff > 0 {
		opts.MaxRetryBackoff = cfg.MaxRetryBackoff
	}
	if cfg.TLSEnabled {
		var tlsConfig *tls.Config
		var err error
		if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		opts.TLSConfig = tlsConfig
	}

	return goredis.NewClient(opts), nil
}

func NewCluster(cfg config.Redis) (*goredis.ClusterClient, error) {
	opts := &goredis.ClusterOptions{
		Addrs:    cfg.Addrs,
		Password: cfg.Password,
	}
	if cfg.PoolSize > 0 {
		opts.PoolSize = cfg.PoolSize
	}
	if cfg.DialTimeout > 0 {
		opts.DialTimeout = cfg.DialTimeout
	}
	if cfg.ReadTimeout > 0 {
		opts.ReadTimeout = cfg.ReadTimeout
	}
	if cfg.WriteTimeout > 0 {
		opts.WriteTimeout = cfg.WriteTimeout
	}
	if cfg.MinIdleConns > 0 {
		opts.MinIdleConns = cfg.MinIdleConns
	}
	if cfg.MaxRetries > 0 {
		opts.MaxRetries = cfg.MaxRetries
	}
	if cfg.MinRetryBackoff > 0 {
		opts.MinRetryBackoff = cfg.MinRetryBackoff
	}
	if cfg.MaxRetryBackoff > 0 {
		opts.MaxRetryBackoff = cfg.MaxRetryBackoff
	}
	if cfg.TLSEnabled {
		var tlsConfig *tls.Config
		var err error
		if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		opts.TLSConfig = tlsConfig
	}

	return goredis.NewClusterClient(opts), nil
}

// Client returns the initialized Redis client handle, standalone or
// cluster depending on configuration.
//
// It reports ErrRedisIsDisabled whenever no handle is held, which covers a
// deployment that never enabled Redis, one whose Init failed, and a process
// past Close. Every operation in this package goes through it, so an absent
// handle is an error at the call site instead of a zero value the caller
// cannot tell from a real answer.
func Client() (goredis.UniversalClient, error) {
	mu.RLock()
	defer mu.RUnlock()
	if cli == nil {
		return nil, ErrRedisIsDisabled
	}
	return cli, nil
}

func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if cli == nil {
		return nil
	}
	err := cli.Close()
	cli = nil
	if err != nil {
		return errors.Wrap(err, "failed to close redis client")
	}
	zap.S().Infow("successfully close redis client", "cluster_mode", config.App.Redis.ClusterMode)
	return nil
}

// Set set any data into redis with specific key.
// If the data type is custom type or structure, you must implement the interface encoding.BinaryMarshaler.
func Set(ctx context.Context, key string, data any, expiration ...time.Duration) error {
	client, err := Client()
	if err != nil {
		return err
	}
	_expiration := config.App.Redis.Expiration
	if len(expiration) > 0 {
		_expiration = expiration[0]
	}
	return client.Set(ctx, redisKey(key), data, _expiration).Err()
}

// SetM set types.Model into redis with specific key.
func SetM[M types.Model](ctx context.Context, key string, m M, expiration ...time.Duration) error {
	client, err := Client()
	if err != nil {
		return err
	}
	_expiration := config.App.Redis.Expiration
	if len(expiration) > 0 {
		_expiration = expiration[0]
	}
	return client.Set(ctx, redisKey(key), modelMarshaler[M]{Model: m}, _expiration).Err()
}

// SetML set one or multiple types.Model into redis with specific key.
func SetML[M types.Model](ctx context.Context, key string, ml []M, expiration ...time.Duration) error {
	client, err := Client()
	if err != nil {
		return err
	}
	_expiration := config.App.Redis.Expiration
	if len(expiration) > 0 {
		_expiration = expiration[0]
	}
	bl := make([]modelMarshaler[M], 0)
	for i := range ml {
		bl = append(bl, modelMarshaler[M]{Model: ml[i]})
	}
	return client.Set(ctx, redisKey(key), modelMarshalerList[M](bl), _expiration).Err()
}

// Get will get raw cache([]byte) from redis.
func Get(ctx context.Context, key string) (cache []byte, err error) {
	client, err := Client()
	if err != nil {
		return nil, err
	}
	cache, err = client.Get(ctx, redisKey(key)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, ErrKeyNotExists
		}
		return nil, err
	}
	return cache, nil
}

// GetInt get cache from redis and decode into integer.
func GetInt(ctx context.Context, key string) (int64, error) {
	client, err := Client()
	if err != nil {
		return 0, err
	}
	cache, err := client.Get(ctx, redisKey(key)).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return 0, ErrKeyNotExists
		}
		return 0, err
	}
	val, err := strconv.Atoi(cache)
	if err != nil {
		return 0, err
	}
	return int64(val), nil
}

// GetM will get cache from redis and decode into types.Model.
func GetM[M types.Model](ctx context.Context, key string) (M, error) {
	client, err := Client()
	if err != nil {
		return *new(M), err
	}
	data, err := client.Get(ctx, redisKey(key)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return *new(M), ErrKeyNotExists
		}
		zap.S().Error(err)
		return *new(M), err
	}
	typ := reflect.TypeOf(*new(M)).Elem()
	val := reflect.New(typ).Interface().(M) //nolint:errcheck
	if err := json.Unmarshal(data, val); err != nil {
		zap.S().Error(err)
		return *new(M), err
	}
	return val, nil
}

// GetML will get cache from redis and decode into []types.Model.
func GetML[M types.Model](ctx context.Context, key string) ([]M, error) {
	client, err := Client()
	if err != nil {
		return nil, err
	}
	data, err := client.Get(ctx, redisKey(key)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, ErrKeyNotExists
		}
		zap.S().Error(err)
		return nil, err
	}

	dest := make([]modelMarshaler[M], 0)
	if err := json.Unmarshal(data, &dest); err != nil {
		zap.S().Error(err)
		return nil, err
	}
	ml := make([]M, 0)
	for i := range dest {
		ml = append(ml, dest[i].Model)
	}
	return ml, nil
}

func Del(ctx context.Context, keys ...string) error {
	client, err := Client()
	if err != nil {
		return err
	}
	return client.Del(ctx, redisKeys(keys)...).Err()
}

// SetNX sets key to value with expiration only when the key does not already exist.
func SetNX(ctx context.Context, key, value string, expiration time.Duration) (bool, error) {
	client, err := Client()
	if err != nil {
		return false, err
	}
	return client.SetNX(ctx, redisKey(key), value, expiration).Result()
}

// Expire updates the ttl for an existing key.
func Expire(ctx context.Context, key string, expiration time.Duration) error {
	client, err := Client()
	if err != nil {
		return err
	}
	return client.Expire(ctx, redisKey(key), expiration).Err()
}

// Incr increments the integer at key by one and returns the new value,
// creating the key at zero first when it does not exist.
//
// The read and the write are one Redis operation, which is what makes it usable
// as a counter under concurrency: a caller that instead read, added, and wrote
// back would lose increments to every interleaving of two requests.
func Incr(ctx context.Context, key string) (int64, error) {
	client, err := Client()
	if err != nil {
		return 0, err
	}
	return client.Incr(ctx, redisKey(key)).Result()
}

// TTL reports the remaining ttl of a key.
//
// Redis answers the two "nothing to report" cases with negative durations
// instead of an error, and they are passed through unchanged: TTLKeyNotExists
// for a key that is not there, TTLNoExpiry for a key that is there and never
// expires. A caller comparing the result against a deadline has to rule both
// out first, because either one compares as less than any positive duration.
func TTL(ctx context.Context, key string) (time.Duration, error) {
	client, err := Client()
	if err != nil {
		return 0, err
	}
	return client.TTL(ctx, redisKey(key)).Result()
}

// ZAdd adds one or multiple string members with the same score into a sorted set.
func ZAdd(ctx context.Context, key string, score float64, members ...string) error {
	client, err := Client()
	if err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}
	entries := make([]goredis.Z, 0, len(members))
	for i := range members {
		entries = append(entries, goredis.Z{Score: score, Member: members[i]})
	}
	return client.ZAdd(ctx, redisKey(key), entries...).Err()
}

// ZRange returns sorted set members in ascending score order.
func ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	client, err := Client()
	if err != nil {
		return nil, err
	}
	return client.ZRange(ctx, redisKey(key), start, stop).Result()
}

// ZRangeByScore returns sorted set members whose scores are between minScore and maxScore.
func ZRangeByScore(ctx context.Context, key, minScore, maxScore string) ([]string, error) {
	client, err := Client()
	if err != nil {
		return nil, err
	}
	args := goredis.ZRangeArgs{
		Key:     redisKey(key),
		Start:   minScore,
		Stop:    maxScore,
		ByScore: true,
	}
	return client.ZRangeArgs(ctx, args).Result()
}

// ZRem removes one or multiple members from a sorted set.
func ZRem(ctx context.Context, key string, members ...string) error {
	client, err := Client()
	if err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}
	memberArgs := make([]any, 0, len(members))
	for i := range members {
		memberArgs = append(memberArgs, members[i])
	}
	return client.ZRem(ctx, redisKey(key), memberArgs...).Err()
}

// ZRemRangeByScore removes sorted set members whose score is between minScore and maxScore.
func ZRemRangeByScore(ctx context.Context, key, minScore, maxScore string) error {
	client, err := Client()
	if err != nil {
		return err
	}
	return client.ZRemRangeByScore(ctx, redisKey(key), minScore, maxScore).Err()
}

// RemovePrefix will scan and delete all redis key that matchs the `prefix`.
// for example: myprefix*
func RemovePrefix(ctx context.Context, prefix string) (err error) {
	client, err := Client()
	if err != nil {
		return err
	}
	iter := client.Scan(ctx, 0, redisPattern(prefix), 0).Iterator()
	for iter.Next(ctx) {
		err = client.Del(ctx, iter.Val()).Err()
		if err != nil {
			zap.S().Error(err)
			return err
		}
	}
	if err := iter.Err(); err != nil {
		zap.S().Error(err)
		return err
	}
	return nil
}

// modelMarshaler
// The receiver of MarshalBinary and UnmarshalBinary must not be a pointer, otherwise redis reports:
// redis: can't marshal redis.modelMarshaler[*myproject/model.Sample] (implement encoding.BinaryMarshaler)
//
// The receiver of MarshalJSON and UnmarshalJSON must be a pointer, otherwise it panics.
type modelMarshaler[M types.Model] struct {
	Model M
}

func (b modelMarshaler[M]) MarshalBinary() ([]byte, error) {
	return json.Marshal(b.Model)
	// buf := new(bytes.Buffer)
	// if err := gob.NewEncoder(buf).Encode(b.Model); err != nil {
	// 	zap.S().Error(err)
	// 	return nil, err
	// }
	// return buf.Bytes(), nil
}

// func (b modelMarshaler[M]) UnmarshalBinary(data []byte) error {
// 	return json.Unmarshal(data, b.Model)
// }

// func (b *modelMarshaler[M]) MarshalJSON() ([]byte, error) {
// 	data, err := json.Marshal(b.Model)
// 	if err != nil {
// 		zap.S().Error(err)
// 		return nil, err
// 	}
// 	return data, err
// }

func (b *modelMarshaler[M]) UnmarshalJSON(data []byte) error {
	if reflect.DeepEqual(b.Model, *new(M)) {
		b.Model = reflect.New(reflect.TypeOf(*new(M)).Elem()).Interface().(M) //nolint:errcheck
	}
	if err := json.Unmarshal(data, &b.Model); err != nil {
		zap.S().Error(err)
		return err
	}
	return nil
}

// modelMarshalerList
// The receiver of MarshalBinary must never be a pointer.
type modelMarshalerList[M types.Model] []modelMarshaler[M]

func (bl modelMarshalerList[M]) MarshalBinary() ([]byte, error) {
	// ml := make([]types.Model, 0)
	// for i := range bl {
	// 	ml = append(ml, bl[i].Model)
	// }
	// return json.Marshal(ml)

	ml := make([]types.Model, len(bl))
	for i := range bl {
		ml[i] = bl[i].Model
	}
	return json.Marshal(ml)
}

// func (bl modelMarshalerList[M]) MarshalJSON() ([]byte, error) {
// 	ml := make([]types.Model, 0)
// 	for i := range bl {
// 		ml = append(ml, bl[i].Model)
// 	}
// 	return json.Marshal(ml)
// }
// func (bl *modelMarshalerList[M]) UnmarshalJSON(data []byte) error {
// 	bs := make([]modelMarshaler[M], 0)
// 	if err := json.Unmarshal(data, &bs); err != nil {
// 		zap.S().Error(err)
// 		return err
// 	}
// 	*bl = modelMarshalerList[M](bs)
// 	return nil
// }
