package dcache_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/hydroan/gst/dcache"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLocalCacheBasicOperations 测试基本的缓存操作
func TestLocalCacheBasicOperations(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)
	assert.NotNil(t, cache)

	// 测试Set和Get
	err = cache.Set("key1", "value1", 1*time.Hour)
	require.NoError(t, err)

	val, err := cache.Get("key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	// 测试Exists
	assert.True(t, cache.Exists("key1"))
	assert.False(t, cache.Exists("nonexistent"))

	// 测试Delete
	err = cache.Delete("key1")
	require.NoError(t, err)
	assert.False(t, cache.Exists("key1"))

	// 测试获取已删除的键
	_, err = cache.Get("key1")
	require.Error(t, err)
	assert.Equal(t, types.ErrEntryNotFound, err)
}

// TestLocalCacheTTL 测试TTL功能
func TestLocalCacheTTL(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// 设置短TTL
	err = cache.Set("ttl-key", "ttl-value", 100*time.Millisecond)
	require.NoError(t, err)

	// 立即检查应该存在
	assert.True(t, cache.Exists("ttl-key"))
	val, err := cache.Get("ttl-key")
	require.NoError(t, err)
	assert.Equal(t, "ttl-value", val)

	// 等待TTL过期
	time.Sleep(200 * time.Millisecond)

	// TTL过期后检查应该不存在
	assert.False(t, cache.Exists("ttl-key"))
	_, err = cache.Get("ttl-key")
	assert.Equal(t, types.ErrEntryNotFound, err)
}

// TestLocalCacheZeroTTL 测试零TTL（永不过期）
func TestLocalCacheZeroTTL(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// 设置零TTL
	err = cache.Set("zero-ttl", "永不过期", 0)
	require.NoError(t, err)

	// 短暂等待后仍应存在
	time.Sleep(100 * time.Millisecond)
	assert.True(t, cache.Exists("zero-ttl"))
	val, err := cache.Get("zero-ttl")
	require.NoError(t, err)
	assert.Equal(t, "永不过期", val)
}

// TestLocalCacheNegativeTTL 测试负TTL（应该被拒绝）
func TestLocalCacheNegativeTTL(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// 设置负TTL
	err = cache.Set("negative-ttl", "invalid", -1*time.Second)
	// 不确定ristretto如何处理负TTL，需要根据实际行为调整断言
	if err != nil {
		assert.Contains(t, err.Error(), "rejected")
	} else {
		// 如果没有错误，检查值是否被设置
		exists := cache.Exists("negative-ttl")
		assert.False(t, exists, "负TTL的键不应该被设置")
	}
}

// TestLocalCacheDifferentTypes 测试不同类型
func TestLocalCacheDifferentTypes(t *testing.T) {
	// 字符串缓存
	strCache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)
	err = strCache.Set("str", "string-value", 1*time.Hour)
	require.NoError(t, err)

	// 整数缓存
	intCache, err := dcache.NewLocalCache[int]()
	require.NoError(t, err)
	err = intCache.Set("int", 42, 1*time.Hour)
	require.NoError(t, err)

	// 结构体缓存 - 使用包级别的 Person 类型
	personCache, err := dcache.NewLocalCache[Person]()
	require.NoError(t, err)
	err = personCache.Set("person", Person{Name: "Alice", Age: 30}, 1*time.Hour)
	require.NoError(t, err)

	// 检查各自类型的值
	strVal, err := strCache.Get("str")
	require.NoError(t, err)
	assert.Equal(t, "string-value", strVal)

	intVal, err := intCache.Get("int")
	require.NoError(t, err)
	assert.Equal(t, 42, intVal)

	personVal, err := personCache.Get("person")
	require.NoError(t, err)
	assert.Equal(t, Person{Name: "Alice", Age: 30}, personVal)
}

// TestLocalCacheOverwrite 测试覆盖已有键
func TestLocalCacheOverwrite(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// 首次设置
	err = cache.Set("overwrite", "original", 1*time.Hour)
	require.NoError(t, err)

	// 覆盖
	err = cache.Set("overwrite", "updated", 2*time.Hour)
	require.NoError(t, err)

	val, err := cache.Get("overwrite")
	require.NoError(t, err)
	assert.Equal(t, "updated", val)
}

// TestLocalCacheConcurrency 测试并发操作
func TestLocalCacheConcurrency(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// 同时进行多个Set和Get操作
	const goroutines = 100
	errCh := make(chan error, goroutines)

	for i := range goroutines {
		go func(idx int) {
			key := fmt.Sprintf("key-%d", idx)
			value := fmt.Sprintf("value-%d", idx)

			err := cache.Set(key, value, 1*time.Hour)
			if err != nil {
				errCh <- err
				return
			}

			val, err := cache.Get(key)
			if err != nil {
				errCh <- err
				return
			}
			if value != val {
				errCh <- fmt.Errorf("expected %q, got %q", value, val)
				return
			}

			errCh <- nil
		}(i)
	}

	// 等待所有goroutine完成
	for range goroutines {
		require.NoError(t, <-errCh)
	}
}

// TestLocalCacheLargeValues 测试大型值
func TestLocalCacheLargeValues(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// 创建一个大字符串
	largeValue := make([]byte, 1<<20) // 1MB
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}
	largeString := string(largeValue)

	// 设置大值
	err = cache.Set("large", largeString, 1*time.Hour)
	require.NoError(t, err)

	// 获取并验证
	val, err := cache.Get("large")
	require.NoError(t, err)
	assert.Equal(t, largeString, val)
}

// TestLocalCacheKeyCollision 测试哈希冲突处理
func TestLocalCacheKeyCollision(t *testing.T) {
	// 注意：这个测试主要是概念性的，因为很难在实际中制造哈希冲突
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// 设置大量随机键以增加冲突可能性
	const keyCount = 10000
	for i := range keyCount {
		key := fmt.Sprintf("collision-test-key-%d", i)
		value := fmt.Sprintf("value-%d", i)
		err := cache.Set(key, value, 1*time.Hour)
		require.NoError(t, err)
	}

	// 随机检查一些键值对
	for i := range 100 {
		idx := i * 100
		key := fmt.Sprintf("collision-test-key-%d", idx)
		expectedValue := fmt.Sprintf("value-%d", idx)

		val, err := cache.Get(key)
		require.NoError(t, err)
		assert.Equal(t, expectedValue, val)
	}
}

// TestLocalCacheMetrics 测试指标收集
func TestLocalCacheMetrics(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// 类型断言为指标提供者
	metricsProvider, ok := cache.(dcache.CacheMetricsProvider)
	assert.True(t, ok, "缓存应该实现cacheMetricsProvider接口")

	// 执行一些操作以生成指标
	for i := range 100 {
		key := fmt.Sprintf("metrics-key-%d", i)
		err := cache.Set(key, fmt.Sprintf("val-%d", i), 1*time.Hour)
		require.NoError(t, err)
	}

	// 一些读取操作
	for i := range 50 {
		key := fmt.Sprintf("metrics-key-%d", i)
		_, err := cache.Get(key)
		require.NoError(t, err)
	}

	// 一些缓存未命中
	for i := 100; i < 150; i++ {
		key := fmt.Sprintf("nonexistent-key-%d", i)
		_, err := cache.Get(key)
		require.Error(t, err)
	}

	// 检查指标
	metrics := metricsProvider.Metrics()
	assert.NotNil(t, metrics)
	assert.Positive(t, metrics.KeysAdded, "应该有键被添加")
	assert.Positive(t, metrics.Misses, "应该有缓存未命中")
}

// TestLocalCacheSingletonBehavior 测试缓存单例行为
func TestLocalCacheSingletonBehavior(t *testing.T) {
	// 创建两个相同类型的缓存
	cache1, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)
	cache2, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// 它们应该是同一个实例
	assert.Equal(t, fmt.Sprintf("%p", cache1), fmt.Sprintf("%p", cache2))

	// 在cache1中设置一个值
	err = cache1.Set("singleton-test", "value", 1*time.Hour)
	require.NoError(t, err)

	// 应该能从cache2中获取
	val, err := cache2.Get("singleton-test")
	require.NoError(t, err)
	assert.Equal(t, "value", val)

	// 创建不同类型的缓存
	intCache, err := dcache.NewLocalCache[int]()
	require.NoError(t, err)

	// 应该是不同的实例
	assert.NotEqual(t, fmt.Sprintf("%p", cache1), fmt.Sprintf("%p", intCache))
}

// // TestLocalCacheRejectedSet 测试缓存拒绝设置操作
// func TestLocalCacheRejectedSet(t *testing.T) {
// 	// 这个测试很难直接实现，因为我们很难强制ristretto拒绝设置操作
// 	// 但我们可以尝试设置大量数据来增加被拒绝的可能性
//
// 	cache, err := NewLocalCache[string]()
// 	assert.NoError(t, err)
//
// 	// 设置大量数据
// 	rejected := false
// 	for i := 0; i < 10000000 && !rejected; i++ {
// 		key := fmt.Sprintf("stress-test-key-%d", i)
// 		value := fmt.Sprintf("value-%d", i)
// 		err := cache.Set(key, value, 1*time.Hour)
// 		if err != nil && err.Error() == "cache rejected the set operation" {
// 			rejected = true
// 		}
// 	}
//
// 	// 注意：不强制断言rejected为true，因为这取决于系统资源和ristretto的内部实现
// 	t.Logf("Set操作被拒绝: %v", rejected)
// }

// TestLocalCacheNilValue 测试nil值处理
func TestLocalCacheNilValue(t *testing.T) {
	cache, err := dcache.NewLocalCache[*string]()
	require.NoError(t, err)

	// 设置nil值
	err = cache.Set("nil-key", nil, 1*time.Hour)
	require.NoError(t, err)

	// 获取nil值
	val, err := cache.Get("nil-key")
	require.NoError(t, err)
	assert.Nil(t, val)
}

// TestLocalCacheEmptyKey 测试空键
func TestLocalCacheEmptyKey(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// 设置空键
	err = cache.Set("", "empty-key-value", 1*time.Hour)
	require.NoError(t, err)

	// 获取空键
	val, err := cache.Get("")
	require.NoError(t, err)
	assert.Equal(t, "empty-key-value", val)
}
