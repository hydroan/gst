package dcache_test

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/dcache"
	"github.com/hydroan/gst/internal/testcontainer"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

type Person struct {
	Name string
	Age  int
}

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests prepares the redis the distributed cache is backed by. os.Exit in
// TestMain would skip the deferred release, hence the wrapper.
func runTests(m *testing.M) int {
	// Before config.Init: the container publishes its address through the
	// environment, which is what config reads.
	release, err := testcontainer.SetupRedis()
	if err != nil {
		panic(err)
	}
	defer func() { _ = release() }()

	if err := config.Init(); err != nil {
		panic(err)
	}
	if err := zap.Init(); err != nil {
		panic(err)
	}
	if err := dcache.Init(); err != nil {
		panic(err)
	}

	return m.Run()
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

// TestDistributedCacheBasicOperations tests the basic operations.
func TestDistributedCacheBasicOperations(t *testing.T) {
	// the test replaces a few dependencies, so the distributedCache under test
	// is built through a dedicated helper
	dc := setupTestDistributedCache[string](t)

	// Set
	err := dc.Set(t.Context(), "test-key", "test-value", 1*time.Minute)
	require.NoError(t, err)

	// the local cache must hold the entry now
	val, err := dc.Get(t.Context(), "test-key")
	require.NoError(t, err)
	require.Equal(t, "test-value", val)

	// Delete
	err = dc.Delete(t.Context(), "test-key")
	require.NoError(t, err)

	// it must be gone
	require.False(t, dc.Exists(t.Context(), "test-key"))

	// a key that does not exist
	_, err = dc.Get(t.Context(), "non-existent")
	require.Error(t, err)
	require.True(t, errors.Is(err, types.ErrEntryNotFound))
}

// TestDistributedCacheWithSync tests the operations that synchronize.
func TestDistributedCacheWithSync(t *testing.T) {
	dc := setupTestDistributedCache[string](t)
	key, value := "test-key", "test-value"
	localTTL, remoteTTL := 500*time.Millisecond, 1*time.Minute

	// SetWithSync
	err := dc.SetWithSync(t.Context(), key, value, localTTL, remoteTTL)
	require.NoError(t, err)

	// GetWithSync (served from the local cache)
	val, err := dc.Get(t.Context(), "test-key")
	require.NoError(t, err)
	require.Equal(t, value, val)

	// after the automatic expiration it is no longer readable
	time.Sleep(localTTL + 50*time.Millisecond) // add some slack so it has surely expired
	val, err = dc.Get(t.Context(), "test-key")
	require.ErrorIs(t, err, types.ErrEntryNotFound)
	require.Empty(t, val)

	// GetWithSync fails without a real Redis in the test environment,
	// so a value is written first to emulate an entry living in Redis
	err = dc.SetWithSync(t.Context(), key, value, localTTL, remoteTTL)
	require.NoError(t, err)

	// give the set operation a moment to complete
	time.Sleep(100 * time.Millisecond)

	val, err = dc.GetWithSync(t.Context(), key, localTTL)
	require.NoError(t, err)
	require.Equal(t, value, val)

	// explicit Delete
	err = dc.Delete(t.Context(), key)
	require.NoError(t, err)
	val, err = dc.Get(t.Context(), key)
	require.ErrorIs(t, err, types.ErrEntryNotFound)
	require.Empty(t, val)

	// set the value again for the rest of the test
	err = dc.SetWithSync(t.Context(), key, value, localTTL, remoteTTL)
	require.NoError(t, err)

	// wait for the set to complete
	time.Sleep(100 * time.Millisecond)

	val, err = dc.GetWithSync(t.Context(), key, localTTL)
	require.NoError(t, err)
	require.Equal(t, value, val)

	// explicit DeleteWithSync
	err = dc.DeleteWithSync(t.Context(), key)
	require.NoError(t, err)
	val, err = dc.Get(t.Context(), key)
	require.ErrorIs(t, err, types.ErrEntryNotFound)
	require.Empty(t, val)

	// wait for the state node to delete the key from redis
	time.Sleep(500 * time.Millisecond)
	val, err = dc.GetWithSync(t.Context(), key, localTTL)
	require.ErrorIs(t, err, types.ErrEntryNotFound)
	require.Empty(t, val)
}

// TestDistributedCacheTTL tests the TTL handling.
func TestDistributedCacheTTL(t *testing.T) {
	dc := setupTestDistributedCache[string](t)

	// set a very short TTL
	err := dc.Set(t.Context(), "ttl-key", "ttl-value", 100*time.Millisecond)
	require.NoError(t, err)

	// it must be readable right away
	val, err := dc.Get(t.Context(), "ttl-key")
	require.NoError(t, err)
	require.Equal(t, "ttl-value", val)

	// wait for the TTL to expire
	time.Sleep(200 * time.Millisecond)

	// now it must be gone
	_, err = dc.Get(t.Context(), "ttl-key")
	require.Error(t, err)
}

// TestDistributedCacheRemoteTTLValidation tests the validation of remoteTTL.
func TestDistributedCacheRemoteTTLValidation(t *testing.T) {
	dc := setupTestDistributedCache[string](t)

	// an invalid TTL pair (remoteTTL < localTTL)
	err := dc.SetWithSync(t.Context(), "invalid-ttl", "value", 2*time.Hour, 1*time.Hour)
	require.Error(t, err)
}

// TestDistributedCacheConcurrency tests concurrent operations.
func TestDistributedCacheConcurrency(t *testing.T) {
	dc := setupTestDistributedCache[string](t)

	// a wait group to synchronize the goroutines
	var wg sync.WaitGroup
	const numGoroutines = 100
	errCh := make(chan error, numGoroutines)

	// run reads and writes from many goroutines at once
	for i := range numGoroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			key := fmt.Sprintf("concurrent-key-%d", idx)
			value := fmt.Sprintf("value-%d", idx)

			// set the value
			err := dc.Set(t.Context(), key, value, 1*time.Minute)
			if err != nil {
				errCh <- err
				return
			}

			// read the value
			val, err := dc.Get(t.Context(), key)
			if err != nil {
				errCh <- err
				return
			}
			if value != val {
				errCh <- errors.Errorf("expected %q, got %q", value, val)
				return
			}

			// delete the value
			err = dc.Delete(t.Context(), key)
			if err != nil {
				errCh <- err
				return
			}
			errCh <- nil
		}(i)
	}

	// wait for every goroutine to finish
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

// TestDistributedCacheDifferentTypes tests caches of different types.
func TestDistributedCacheDifferentTypes(t *testing.T) {
	// string cache
	strCache := setupTestDistributedCache[string](t)

	// int cache
	intCache := setupTestDistributedCache[int](t)

	personCache := setupTestDistributedCache[Person](t)

	// operate on each type
	err := strCache.Set(t.Context(), "str", "string-value", 1*time.Minute)
	require.NoError(t, err)

	err = intCache.Set(t.Context(), "int", 42, 1*time.Minute)
	require.NoError(t, err)

	err = personCache.Set(t.Context(), "person", Person{Name: "Alice", Age: 30}, 1*time.Minute)
	require.NoError(t, err)

	// check the values
	strVal, err := strCache.Get(t.Context(), "str")
	require.NoError(t, err)
	require.Equal(t, "string-value", strVal)

	intVal, err := intCache.Get(t.Context(), "int")
	require.NoError(t, err)
	require.Equal(t, 42, intVal)

	personVal, err := personCache.Get(t.Context(), "person")
	require.NoError(t, err)
	require.Equal(t, Person{Name: "Alice", Age: 30}, personVal)
}

// TestDistributedCacheLargeValues tests large values.
func TestDistributedCacheLargeValues(t *testing.T) {
	dc := setupTestDistributedCache[string](t)

	// build a large string
	largeValue := make([]byte, 1<<20) // 1MB
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}
	largeString := string(largeValue)

	// store the large value
	err := dc.Set(t.Context(), "large", largeString, 1*time.Hour)
	require.NoError(t, err)

	// read it back and verify
	val, err := dc.Get(t.Context(), "large")
	require.NoError(t, err)
	require.Equal(t, largeString, val)
}

// TestDistributedCacheEdgeCases tests the edge cases.
func TestDistributedCacheEdgeCases(t *testing.T) {
	dc := setupTestDistributedCache[string](t)

	// an empty key
	err := dc.Set(t.Context(), "", "empty-key", 1*time.Hour)
	require.NoError(t, err)
	val, err := dc.Get(t.Context(), "")
	require.NoError(t, err)
	require.Equal(t, "empty-key", val)

	// a zero TTL
	err = dc.Set(t.Context(), "zero-ttl", "forever", 0)
	require.NoError(t, err)

	// a tiny TTL
	err = dc.Set(t.Context(), "tiny-ttl", "quick", 1*time.Nanosecond)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)
	_, err = dc.Get(t.Context(), "tiny-ttl")
	require.Error(t, err) // it must have expired

	// a huge TTL
	err = dc.Set(t.Context(), "huge-ttl", "longterm", 100*365*24*time.Hour) // ~100 years
	require.NoError(t, err)
	val, err = dc.Get(t.Context(), "huge-ttl")
	require.NoError(t, err)
	require.Equal(t, "longterm", val)
}
