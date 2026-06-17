package dcache_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/dcache"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

type Person struct {
	Name string
	Age  int
}

func init() {
	if err := config.Init(); err != nil {
		panic(err)
	}
	if err := zap.Init(); err != nil {
		panic(err)
	}
	if err := dcache.Init(); err != nil {
		panic(err)
	}
}

func setupTestDistributedCache[T any](t *testing.T) types.DistributedCache[T] {
	t.Helper(
	// redisCli, err := redis.New(config.App.Redis)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// redisCache, err := dcache.NewRedisCache[any](context.TODO(), redisCli)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// distributed, err := dcache.NewDistributedCache(
	// 	dcache.WithKafkaBrokers[T]([]string{"127.0.0.1:9092"}),
	// 	dcache.WithRedisCache[T](redisCache),
	// )
	)

	distributed, err := dcache.NewDistributedCache[T]()
	if err != nil {
		t.Fatal(err)
	}

	return distributed
}

// TestDistributedCacheBasicOperations 测试基本操作
func TestDistributedCacheBasicOperations(t *testing.T) {
	// 为了测试，我们需要替换一些依赖组件
	// 这里我们创建一个方法来获取测试用的distributedCache
	dc := setupTestDistributedCache[string](t)

	// 测试Set操作
	err := dc.Set("test-key", "test-value", 1*time.Minute)
	require.NoError(t, err)

	// 本地缓存应该被设置
	val, err := dc.Get("test-key")
	require.NoError(t, err)
	require.Equal(t, "test-value", val)

	// 测试Delete操作
	err = dc.Delete("test-key")
	require.NoError(t, err)

	// 应该不存在了
	require.False(t, dc.Exists("test-key"))

	// 测试不存在的键
	_, err = dc.Get("non-existent")
	require.Error(t, err)
	require.True(t, errors.Is(err, types.ErrEntryNotFound))
}

// TestDistributedCacheWithSync 测试带同步的操作
func TestDistributedCacheWithSync(t *testing.T) {
	dc := setupTestDistributedCache[string](t)
	key, value := "test-key", "test-value"
	localTTL, remoteTTL := 500*time.Millisecond, 1*time.Minute

	// 测试SetWithSync
	err := dc.SetWithSync(key, value, localTTL, remoteTTL)
	require.NoError(t, err)

	// 测试GetWithSync (从本地缓存获取)
	val, err := dc.Get("test-key")
	require.NoError(t, err)
	require.Equal(t, value, val)

	// 自动过期拿不到
	time.Sleep(localTTL + 50*time.Millisecond) // 增加一些缓冲时间确保过期
	val, err = dc.Get("test-key")
	require.ErrorIs(t, err, types.ErrEntryNotFound)
	require.Empty(t, val)

	// 由于测试环境没有真实的 Redis，GetWithSync 会失败
	// 这里我们先设置一个值到 Redis 模拟的场景
	err = dc.SetWithSync(key, value, localTTL, remoteTTL)
	require.NoError(t, err)

	// 等待一小段时间让设置操作完成
	time.Sleep(100 * time.Millisecond)

	val, err = dc.GetWithSync(key, localTTL)
	require.NoError(t, err)
	require.Equal(t, value, val)

	// 主动删除 Delete
	err = dc.Delete(key)
	require.NoError(t, err)
	val, err = dc.Get(key)
	require.ErrorIs(t, err, types.ErrEntryNotFound)
	require.Empty(t, val)

	// 重新设置值用于后续测试
	err = dc.SetWithSync(key, value, localTTL, remoteTTL)
	require.NoError(t, err)

	// 等待设置完成
	time.Sleep(100 * time.Millisecond)

	val, err = dc.GetWithSync(key, localTTL)
	require.NoError(t, err)
	require.Equal(t, value, val)

	// 主动删除 DeleteWithSync
	err = dc.DeleteWithSync(key)
	require.NoError(t, err)
	val, err = dc.Get(key)
	require.ErrorIs(t, err, types.ErrEntryNotFound)
	require.Empty(t, val)

	// 等状态节点删除 redis 中的 key
	time.Sleep(500 * time.Millisecond)
	val, err = dc.GetWithSync(key, localTTL)
	require.ErrorIs(t, err, types.ErrEntryNotFound)
	require.Empty(t, val)
}

// TestDistributedCacheTTL 测试TTL功能
func TestDistributedCacheTTL(t *testing.T) {
	dc := setupTestDistributedCache[string](t)

	// 设置非常短的TTL
	err := dc.Set("ttl-key", "ttl-value", 100*time.Millisecond)
	require.NoError(t, err)

	// 立即应该能获取
	val, err := dc.Get("ttl-key")
	require.NoError(t, err)
	require.Equal(t, "ttl-value", val)

	// 等待TTL过期
	time.Sleep(200 * time.Millisecond)

	// 现在应该获取不到了
	_, err = dc.Get("ttl-key")
	require.Error(t, err)
}

// TestDistributedCacheRemoteTTLValidation 测试RemoteTTL验证
func TestDistributedCacheRemoteTTLValidation(t *testing.T) {
	dc := setupTestDistributedCache[string](t)

	// 设置错误的TTL (remoteTTL < localTTL)
	err := dc.SetWithSync("invalid-ttl", "value", 2*time.Hour, 1*time.Hour)
	require.Error(t, err)
}

// TestDistributedCacheConcurrency 测试并发操作
func TestDistributedCacheConcurrency(t *testing.T) {
	dc := setupTestDistributedCache[string](t)

	// 创建等待组来同步goroutines
	var wg sync.WaitGroup
	const numGoroutines = 100
	errCh := make(chan error, numGoroutines)

	// 启动多个goroutines同时进行读写操作
	for i := range numGoroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			key := fmt.Sprintf("concurrent-key-%d", idx)
			value := fmt.Sprintf("value-%d", idx)

			// 设置值
			err := dc.Set(key, value, 1*time.Minute)
			if err != nil {
				errCh <- err
				return
			}

			// 读取值
			val, err := dc.Get(key)
			if err != nil {
				errCh <- err
				return
			}
			if value != val {
				errCh <- errors.Errorf("expected %q, got %q", value, val)
				return
			}

			// 删除值
			err = dc.Delete(key)
			if err != nil {
				errCh <- err
				return
			}
			errCh <- nil
		}(i)
	}

	// 等待所有goroutines完成
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

// TestDistributedCacheDifferentTypes 测试不同类型的缓存
func TestDistributedCacheDifferentTypes(t *testing.T) {
	// 字符串缓存
	strCache := setupTestDistributedCache[string](t)

	// 整数缓存
	intCache := setupTestDistributedCache[int](t)

	personCache := setupTestDistributedCache[Person](t)

	// 测试各种类型操作
	err := strCache.Set("str", "string-value", 1*time.Minute)
	require.NoError(t, err)

	err = intCache.Set("int", 42, 1*time.Minute)
	require.NoError(t, err)

	err = personCache.Set("person", Person{Name: "Alice", Age: 30}, 1*time.Minute)
	require.NoError(t, err)

	// 检查值
	strVal, err := strCache.Get("str")
	require.NoError(t, err)
	require.Equal(t, "string-value", strVal)

	intVal, err := intCache.Get("int")
	require.NoError(t, err)
	require.Equal(t, 42, intVal)

	personVal, err := personCache.Get("person")
	require.NoError(t, err)
	require.Equal(t, Person{Name: "Alice", Age: 30}, personVal)
}

// TestDistributedCacheLargeValues 测试大型值
func TestDistributedCacheLargeValues(t *testing.T) {
	dc := setupTestDistributedCache[string](t)

	// 创建一个大字符串
	largeValue := make([]byte, 1<<20) // 1MB
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}
	largeString := string(largeValue)

	// 设置大值
	err := dc.Set("large", largeString, 1*time.Hour)
	require.NoError(t, err)

	// 获取并验证
	val, err := dc.Get("large")
	require.NoError(t, err)
	require.Equal(t, largeString, val)
}

// TestDistributedCacheEdgeCases 测试边缘情况
func TestDistributedCacheEdgeCases(t *testing.T) {
	dc := setupTestDistributedCache[string](t)

	// 测试空键
	err := dc.Set("", "empty-key", 1*time.Hour)
	require.NoError(t, err)
	val, err := dc.Get("")
	require.NoError(t, err)
	require.Equal(t, "empty-key", val)

	// 测试零TTL
	err = dc.Set("zero-ttl", "forever", 0)
	require.NoError(t, err)

	// 测试极小TTL
	err = dc.Set("tiny-ttl", "quick", 1*time.Nanosecond)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)
	_, err = dc.Get("tiny-ttl")
	require.Error(t, err) // 应该已经过期

	// 测试极大TTL
	err = dc.Set("huge-ttl", "longterm", 100*365*24*time.Hour) // ~100年
	require.NoError(t, err)
	val, err = dc.Get("huge-ttl")
	require.NoError(t, err)
	require.Equal(t, "longterm", val)
}
