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

// runTests prepares the kafka broker the distributed cache propagates
// through. os.Exit in TestMain would skip the deferred release, hence the
// wrapper.
func runTests(m *testing.M) int {
	// Before config.Init: the container publishes its address through the
	// environment, which is what config reads.
	release, err := testcontainer.SetupKafka()
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

	return m.Run()
}

func setupTestDistributedCache[T any](t *testing.T) types.Cache[T] {
	t.Helper()

	distributed, err := dcache.NewDistributedCache[T]()
	if err != nil {
		t.Fatal(err)
	}

	return distributed
}

// TestDistributedCacheBasicOperations tests the basic operations.
func TestDistributedCacheBasicOperations(t *testing.T) {
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
