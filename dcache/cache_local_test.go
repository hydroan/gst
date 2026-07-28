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

// TestLocalCacheBasicOperations tests the basic cache operations.
func TestLocalCacheBasicOperations(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)
	assert.NotNil(t, cache)

	// Set and Get
	err = cache.Set("key1", "value1", 1*time.Hour)
	require.NoError(t, err)

	val, err := cache.Get("key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	// Exists
	assert.True(t, cache.Exists("key1"))
	assert.False(t, cache.Exists("nonexistent"))

	// Delete
	err = cache.Delete("key1")
	require.NoError(t, err)
	assert.False(t, cache.Exists("key1"))

	// get a key that was deleted
	_, err = cache.Get("key1")
	require.Error(t, err)
	assert.Equal(t, types.ErrEntryNotFound, err)
}

// TestLocalCacheTTL tests the TTL handling.
func TestLocalCacheTTL(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// set a short TTL
	err = cache.Set("ttl-key", "ttl-value", 100*time.Millisecond)
	require.NoError(t, err)

	// it must exist right away
	assert.True(t, cache.Exists("ttl-key"))
	val, err := cache.Get("ttl-key")
	require.NoError(t, err)
	assert.Equal(t, "ttl-value", val)

	// wait for the TTL to expire
	time.Sleep(200 * time.Millisecond)

	// once the TTL expired it must be gone
	assert.False(t, cache.Exists("ttl-key"))
	_, err = cache.Get("ttl-key")
	assert.Equal(t, types.ErrEntryNotFound, err)
}

// TestLocalCacheZeroTTL tests a zero TTL, which never expires.
func TestLocalCacheZeroTTL(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// set a zero TTL
	err = cache.Set("zero-ttl", "永不过期", 0)
	require.NoError(t, err)

	// it must still exist after a short wait
	time.Sleep(100 * time.Millisecond)
	assert.True(t, cache.Exists("zero-ttl"))
	val, err := cache.Get("zero-ttl")
	require.NoError(t, err)
	assert.Equal(t, "永不过期", val)
}

// TestLocalCacheNegativeTTL tests a negative TTL, which must be rejected.
func TestLocalCacheNegativeTTL(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// set a negative TTL
	err = cache.Set("negative-ttl", "invalid", -1*time.Second)
	// how ristretto handles a negative TTL is unclear, so the assertion follows the real behavior
	if err != nil {
		assert.Contains(t, err.Error(), "rejected")
	} else {
		// without an error, check whether the value was stored anyway
		exists := cache.Exists("negative-ttl")
		assert.False(t, exists, "负TTL的键不应该被设置")
	}
}

// TestLocalCacheDifferentTypes tests caches of different types.
func TestLocalCacheDifferentTypes(t *testing.T) {
	// string cache
	strCache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)
	err = strCache.Set("str", "string-value", 1*time.Hour)
	require.NoError(t, err)

	// int cache
	intCache, err := dcache.NewLocalCache[int]()
	require.NoError(t, err)
	err = intCache.Set("int", 42, 1*time.Hour)
	require.NoError(t, err)

	// struct cache, using the package level Person type
	personCache, err := dcache.NewLocalCache[Person]()
	require.NoError(t, err)
	err = personCache.Set("person", Person{Name: "Alice", Age: 30}, 1*time.Hour)
	require.NoError(t, err)

	// check the value of each type
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

// TestLocalCacheOverwrite tests overwriting an existing key.
func TestLocalCacheOverwrite(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// first set
	err = cache.Set("overwrite", "original", 1*time.Hour)
	require.NoError(t, err)

	// overwrite
	err = cache.Set("overwrite", "updated", 2*time.Hour)
	require.NoError(t, err)

	val, err := cache.Get("overwrite")
	require.NoError(t, err)
	assert.Equal(t, "updated", val)
}

// TestLocalCacheConcurrency tests concurrent operations.
func TestLocalCacheConcurrency(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// run many Set and Get operations at once
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

	// wait for every goroutine to finish
	for range goroutines {
		require.NoError(t, <-errCh)
	}
}

// TestLocalCacheLargeValues tests large values.
func TestLocalCacheLargeValues(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// build a large string
	largeValue := make([]byte, 1<<20) // 1MB
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}
	largeString := string(largeValue)

	// store the large value
	err = cache.Set("large", largeString, 1*time.Hour)
	require.NoError(t, err)

	// read it back and verify
	val, err := cache.Get("large")
	require.NoError(t, err)
	assert.Equal(t, largeString, val)
}

// TestLocalCacheKeyCollision tests how hash collisions are handled.
func TestLocalCacheKeyCollision(t *testing.T) {
	// NOTE: this test is mostly conceptual, because forcing a real hash collision is hard.
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// store many keys to raise the chance of a collision
	const keyCount = 10000
	for i := range keyCount {
		key := fmt.Sprintf("collision-test-key-%d", i)
		value := fmt.Sprintf("value-%d", i)
		err := cache.Set(key, value, 1*time.Hour)
		require.NoError(t, err)
	}

	// spot check some of the key value pairs
	for i := range 100 {
		idx := i * 100
		key := fmt.Sprintf("collision-test-key-%d", idx)
		expectedValue := fmt.Sprintf("value-%d", idx)

		val, err := cache.Get(key)
		require.NoError(t, err)
		assert.Equal(t, expectedValue, val)
	}
}

// TestLocalCacheMetrics tests the metrics collection.
func TestLocalCacheMetrics(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// assert the cache is a metrics provider
	metricsProvider, ok := cache.(dcache.CacheMetricsProvider)
	assert.True(t, ok, "缓存应该实现cacheMetricsProvider接口")

	// run some operations so that metrics are produced
	for i := range 100 {
		key := fmt.Sprintf("metrics-key-%d", i)
		err := cache.Set(key, fmt.Sprintf("val-%d", i), 1*time.Hour)
		require.NoError(t, err)
	}

	// some reads
	for i := range 50 {
		key := fmt.Sprintf("metrics-key-%d", i)
		_, err := cache.Get(key)
		require.NoError(t, err)
	}

	// some cache misses
	for i := 100; i < 150; i++ {
		key := fmt.Sprintf("nonexistent-key-%d", i)
		_, err := cache.Get(key)
		require.Error(t, err)
	}

	// check the metrics
	metrics := metricsProvider.Metrics()
	assert.NotNil(t, metrics)
	assert.Positive(t, metrics.KeysAdded, "应该有键被添加")
	assert.Positive(t, metrics.Misses, "应该有缓存未命中")
}

// TestLocalCacheSingletonBehavior tests that one cache per type is shared.
func TestLocalCacheSingletonBehavior(t *testing.T) {
	// create two caches of the same type
	cache1, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)
	cache2, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// they must be the same instance
	assert.Equal(t, fmt.Sprintf("%p", cache1), fmt.Sprintf("%p", cache2))

	// set a value through cache1
	err = cache1.Set("singleton-test", "value", 1*time.Hour)
	require.NoError(t, err)

	// it must be readable through cache2
	val, err := cache2.Get("singleton-test")
	require.NoError(t, err)
	assert.Equal(t, "value", val)

	// create a cache of another type
	intCache, err := dcache.NewLocalCache[int]()
	require.NoError(t, err)

	// it must be a different instance
	assert.NotEqual(t, fmt.Sprintf("%p", cache1), fmt.Sprintf("%p", intCache))
}

// // TestLocalCacheRejectedSet tests a set operation the cache rejects.
// func TestLocalCacheRejectedSet(t *testing.T) {
// 	// this test is hard to write directly, because forcing ristretto to reject a set is hard,
// 	// but storing a lot of data raises the chance of a rejection
//
// 	cache, err := NewLocalCache[string]()
// 	assert.NoError(t, err)
//
// 	// store a lot of data
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
// 	// NOTE: rejected is not asserted to be true, it depends on the system resources and on ristretto internals
// 	t.Logf("set operation rejected: %v", rejected)
// }

// TestLocalCacheNilValue tests how a nil value is handled.
func TestLocalCacheNilValue(t *testing.T) {
	cache, err := dcache.NewLocalCache[*string]()
	require.NoError(t, err)

	// store a nil value
	err = cache.Set("nil-key", nil, 1*time.Hour)
	require.NoError(t, err)

	// read the nil value back
	val, err := cache.Get("nil-key")
	require.NoError(t, err)
	assert.Nil(t, val)
}

// TestLocalCacheEmptyKey tests an empty key.
func TestLocalCacheEmptyKey(t *testing.T) {
	cache, err := dcache.NewLocalCache[string]()
	require.NoError(t, err)

	// store an entry under the empty key
	err = cache.Set("", "empty-key-value", 1*time.Hour)
	require.NoError(t, err)

	// read the empty key back
	val, err := cache.Get("")
	require.NoError(t, err)
	assert.Equal(t, "empty-key-value", val)
}
