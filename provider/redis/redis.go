package redis

// 如果使用 Redis 版本小于等于 6，安装 v8 版本
// 如果使用 Redis 版本大于等于 7，安装 v9 版本

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
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	jsoniter "github.com/json-iterator/go"
	redisotel "github.com/redis/go-redis/extra/redisotel/v9"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

var (
	client      *goredis.Client
	cluster     *goredis.ClusterClient
	cli         goredis.UniversalClient
	mu          sync.Mutex
	initialized bool

	ErrKeyNotExists    = errors.New("key no loger exists, may be expired")
	ErrRedisIsDisabled = errors.New("redis is disabled")
)

func redisKey(key string) string {
	namespace := strings.Trim(config.App.Redis.Namespace, ": ")
	if namespace == "" || strings.HasPrefix(key, namespace+":") {
		return key
	}
	return namespace + ":" + key
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
	if !cfg.Enable {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	if cfg.ClusterMode {
		if cluster, err = NewCluster(cfg); err != nil {
			return errors.Wrap(err, "failed to connect to redis")
		}
		cli = cluster
		zap.S().Infow("successfully connect to redis", "addrs", cfg.Addrs, "cluster_mode", cfg.ClusterMode)
		return nil
	}
	if client, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to redis")
	}
	cli = client
	zap.S().Infow("successfully connect to redis", "addr", cfg.Addr, "db", cfg.DB, "cluster_mode", cfg.ClusterMode)

	if err = cli.Set(context.TODO(), cfg.Namespace+"_"+"now", time.Now().Format(consts.DATE_TIME_LAYOUT), cfg.Expiration).Err(); err != nil {
		zap.S().Error(err)
		cli.Close()
		client = nil
		cluster = nil
		return errors.Wrap(err, "failed to set redis key "+cfg.Namespace+"_"+"now")
	}
	if err = errors.Join(redisotel.InstrumentTracing(cli), redisotel.InstrumentMetrics(cli)); err != nil {
		zap.S().Error(err)
		cli.Close()
		client = nil
		cluster = nil
		return err
	}

	initialized = true
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
	if cfg.EnableTLS {
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
	if cfg.EnableTLS {
		var tlsConfig *tls.Config
		var err error
		if tlsConfig, err = util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify); err != nil {
			return nil, errors.Wrap(err, "failed to build TLS config")
		}
		opts.TLSConfig = tlsConfig
	}

	return goredis.NewClusterClient(opts), nil
}

func Close() {
	if client != nil {
		if err := client.Close(); err != nil {
			zap.S().Errorw("failed to close redis client", "error", err)
		} else {
			zap.S().Infow("successfully close redis client")
		}
		cli = nil
		client = nil
	}

	if cluster != nil {
		if err := cluster.Close(); err != nil {
			zap.S().Errorw("failed to close redis cluster client", "error", err)
		} else {
			zap.S().Infow("successfully close redis cluster client")
		}
		cli = nil
		cluster = nil
	}
}

// Set set any data into redis with specific key.
// If the data type is custom type or structure, you must implement the interface encoding.BinaryMarshaler.
func Set(ctx context.Context, key string, data any, expiration ...time.Duration) error {
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return nil
	}
	_expiration := config.App.Redis.Expiration
	if len(expiration) > 0 {
		_expiration = expiration[0]
	}
	key = redisKey(key)
	if config.App.Redis.ClusterMode {
		return cluster.Set(ctx, key, data, _expiration).Err()
	}
	return client.Set(ctx, key, data, _expiration).Err()
}

// SetM set types.Model into redis with specific key.
func SetM[M types.Model](ctx context.Context, key string, m M, expiration ...time.Duration) error {
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return nil
	}
	_expiration := config.App.Redis.Expiration
	if len(expiration) > 0 {
		_expiration = expiration[0]
	}
	key = redisKey(key)
	if config.App.Redis.ClusterMode {
		return cluster.Set(ctx, key, modelMarshaler[M]{Model: m}, _expiration).Err()
	}
	return client.Set(ctx, key, modelMarshaler[M]{Model: m}, _expiration).Err()
}

// SetML set one or multiple types.Model into redis with specific key.
func SetML[M types.Model](ctx context.Context, key string, ml []M, expiration ...time.Duration) error {
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return nil
	}
	_expiration := config.App.Redis.Expiration
	if len(expiration) > 0 {
		_expiration = expiration[0]
	}
	bl := make([]modelMarshaler[M], 0)
	for i := range ml {
		bl = append(bl, modelMarshaler[M]{Model: ml[i]})
	}
	key = redisKey(key)
	if config.App.Redis.ClusterMode {
		return cluster.Set(ctx, key, modelMarshalerList[M](bl), _expiration).Err()
	}
	return client.Set(ctx, key, modelMarshalerList[M](bl), _expiration).Err()
}

// Get will get raw cache([]byte) from redis.
func Get(ctx context.Context, key string) (cache []byte, err error) {
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return make([]byte, 0), nil
	}
	key = redisKey(key)
	if config.App.Redis.ClusterMode {
		cache, err = cluster.Get(ctx, key).Bytes()
	} else {
		cache, err = client.Get(ctx, key).Bytes()
	}
	if errors.Is(err, goredis.Nil) {
		return nil, ErrKeyNotExists
	}
	return cache, nil
}

// GetInt get cache from redis and decode into integer.
func GetInt(ctx context.Context, key string) (int64, error) {
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return 0, nil
	}
	key = redisKey(key)
	var cache string
	var err error
	if config.App.Redis.ClusterMode {
		cache, err = cluster.Get(ctx, key).Result()
	} else {
		cache, err = client.Get(ctx, key).Result()
	}
	if errors.Is(err, goredis.Nil) {
		return 0, ErrKeyNotExists
	}
	val, err := strconv.Atoi(cache)
	if err != nil {
		return 0, err
	}
	return int64(val), nil
}

// GetM will get cache from redis and decode into types.Model.
func GetM[M types.Model](ctx context.Context, key string) (M, error) {
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return *new(M), nil
	}
	key = redisKey(key)
	var data []byte
	var err error
	if config.App.Redis.ClusterMode {
		data, err = cluster.Get(ctx, key).Bytes()
	} else {
		data, err = client.Get(ctx, key).Bytes()
	}
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
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return make([]M, 0), nil
	}
	key = redisKey(key)
	var data []byte
	var err error
	if config.App.Redis.ClusterMode {
		data, err = cluster.Get(ctx, key).Bytes()
	} else {
		data, err = client.Get(ctx, key).Bytes()
	}
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, ErrKeyNotExists
		}
		zap.S().Error(err)
		return nil, err
	}
	// typ := reflect.TypeOf(*new(M)).Elem()

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
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return nil
	}
	keys = redisKeys(keys)
	if config.App.Redis.ClusterMode {
		return cluster.Del(ctx, keys...).Err()
	}
	return client.Del(ctx, keys...).Err()
}

// Expire updates the ttl for an existing key.
func Expire(ctx context.Context, key string, expiration time.Duration) error {
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return nil
	}
	key = redisKey(key)
	if config.App.Redis.ClusterMode {
		return cluster.Expire(ctx, key, expiration).Err()
	}
	return client.Expire(ctx, key, expiration).Err()
}

// ZAdd adds one or multiple string members with the same score into a sorted set.
func ZAdd(ctx context.Context, key string, score float64, members ...string) error {
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return nil
	}
	if len(members) == 0 {
		return nil
	}
	key = redisKey(key)
	entries := make([]goredis.Z, 0, len(members))
	for i := range members {
		entries = append(entries, goredis.Z{Score: score, Member: members[i]})
	}
	if config.App.Redis.ClusterMode {
		return cluster.ZAdd(ctx, key, entries...).Err()
	}
	return client.ZAdd(ctx, key, entries...).Err()
}

// ZRange returns sorted set members in ascending score order.
func ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return make([]string, 0), nil
	}
	key = redisKey(key)
	if config.App.Redis.ClusterMode {
		return cluster.ZRange(ctx, key, start, stop).Result()
	}
	return client.ZRange(ctx, key, start, stop).Result()
}

// ZRem removes one or multiple members from a sorted set.
func ZRem(ctx context.Context, key string, members ...string) error {
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return nil
	}
	if len(members) == 0 {
		return nil
	}
	key = redisKey(key)
	memberArgs := make([]any, 0, len(members))
	for i := range members {
		memberArgs = append(memberArgs, members[i])
	}
	if config.App.Redis.ClusterMode {
		return cluster.ZRem(ctx, key, memberArgs...).Err()
	}
	return client.ZRem(ctx, key, memberArgs...).Err()
}

// ZRemRangeByScore removes sorted set members whose score is between minScore and maxScore.
func ZRemRangeByScore(ctx context.Context, key, minScore, maxScore string) error {
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return nil
	}
	key = redisKey(key)
	if config.App.Redis.ClusterMode {
		return cluster.ZRemRangeByScore(ctx, key, minScore, maxScore).Err()
	}
	return client.ZRemRangeByScore(ctx, key, minScore, maxScore).Err()
}

// RemovePrefix will scan and delete all redis key that matchs the `prefix`.
// for example: myprefix*
func RemovePrefix(ctx context.Context, prefix string) (err error) {
	if !config.App.Redis.Enable {
		zap.S().Warn(ErrRedisIsDisabled.Error())
		return nil
	}
	prefix = redisPattern(prefix)
	iter := cli.Scan(ctx, 0, prefix, 0).Iterator()
	for iter.Next(ctx) {
		err = cli.Del(ctx, iter.Val()).Err()
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
// MarshalBinary, UnmarshalBinary 的 receiver 不能是指针, 否则 redis 会报错:
// redis: can't marshal redis.modelMarshaler[*myproject/model.FeishuUser] (implement encoding.BinaryMarshaler)
//
// MarshalJSON, UnmarshalJSON 的 receiver 必须是指针, 否则 panic
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
// MarshalBinary 的 receiver一定不能是指针
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
