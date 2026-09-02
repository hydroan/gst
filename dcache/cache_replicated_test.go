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
	"github.com/hydroan/gst/internal/testutil/testcontainer"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

type Person struct {
	Name string
	Age  int
}

// Word is JSON-compatible with string on purpose: the cross-type isolation
// test uses the pair to prove the event type filter, not just unmarshal
// failures, keeps types apart.
type Word string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests prepares the kafka broker the replicated cache propagates
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

func setupTestCache[T any](t *testing.T) types.Cache[T] {
	t.Helper()

	replicated, err := dcache.Cache[T]()
	if err != nil {
		t.Fatal(err)
	}

	return replicated
}

// TestCacheBasicOperations tests the basic operations.
func TestCacheBasicOperations(t *testing.T) {
	dc := setupTestCache[string](t)

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

// TestCacheTTL tests the TTL handling.
func TestCacheTTL(t *testing.T) {
	dc := setupTestCache[string](t)

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
	require.ErrorIs(t, err, types.ErrEntryNotFound)
}

// TestCacheConcurrency tests concurrent operations.
func TestCacheConcurrency(t *testing.T) {
	dc := setupTestCache[string](t)

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

// TestCacheDifferentTypes tests caches of different types.
func TestCacheDifferentTypes(t *testing.T) {
	// string cache
	strCache := setupTestCache[string](t)

	// int cache
	intCache := setupTestCache[int](t)

	personCache := setupTestCache[Person](t)

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

	// One key shared across types must stay isolated twice over: the local
	// stores are one per type, and the broadcast events carry the type tag.
	// Word and string are JSON-compatible, so a broken type filter would let
	// one leak into the other once the events loop back. The word cache
	// writes first and the string events are republished with ever newer
	// timestamps afterwards: a leak cannot hide behind the watermark, and
	// the republishing keeps pressing until the word consumer has surely
	// joined.
	wordCache := setupTestCache[Word](t)
	err = wordCache.Set(t.Context(), "shared-key", Word("word-value"), 1*time.Minute)
	require.NoError(t, err)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		require.NoError(t, strCache.Set(t.Context(), "shared-key", "string-value", 1*time.Minute))
		time.Sleep(100 * time.Millisecond)
	}

	strVal, err = strCache.Get(t.Context(), "shared-key")
	require.NoError(t, err)
	require.Equal(t, "string-value", strVal)
	wordVal, err := wordCache.Get(t.Context(), "shared-key")
	require.NoError(t, err)
	require.Equal(t, Word("word-value"), wordVal, "string events must not leak into the word cache")

	// Deleting the key in one type must not touch the other; the string
	// delete carries the newest timestamp of all, so a filter leak would
	// remove the word entry.
	require.NoError(t, strCache.Delete(t.Context(), "shared-key"))
	time.Sleep(time.Second)
	require.False(t, strCache.Exists(t.Context(), "shared-key"))
	wordVal, err = wordCache.Get(t.Context(), "shared-key")
	require.NoError(t, err)
	require.Equal(t, Word("word-value"), wordVal)
}

// TestCacheLargeValueStoresLocally pins the local half of the
// large-value behavior: the store accepts the value and serves it back. The
// broadcast is best-effort — this 1MB value expands beyond the broker's
// default message limit once JSON-escaped, so its event is dropped with an
// error log and the value never reaches the peers, as the package
// documentation declares.
func TestCacheLargeValueStoresLocally(t *testing.T) {
	dc := setupTestCache[string](t)

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

// TestCacheEdgeCases tests the edge cases.
func TestCacheEdgeCases(t *testing.T) {
	dc := setupTestCache[string](t)

	// an empty key
	err := dc.Set(t.Context(), "", "empty-key", 1*time.Hour)
	require.NoError(t, err)
	val, err := dc.Get(t.Context(), "")
	require.NoError(t, err)
	require.Equal(t, "empty-key", val)

	// a zero TTL never expires; reading it back pins the contract the store
	// backend implements with a dedicated branch
	err = dc.Set(t.Context(), "zero-ttl", "forever", 0)
	require.NoError(t, err)
	val, err = dc.Get(t.Context(), "zero-ttl")
	require.NoError(t, err)
	require.Equal(t, "forever", val)

	// the smallest honored TTL: sub-millisecond lifetimes are rejected by
	// the store backend, so a millisecond is the shortest that stores
	err = dc.Set(t.Context(), "tiny-ttl", "quick", time.Millisecond)
	require.NoError(t, err)
	time.Sleep(20 * time.Millisecond)
	_, err = dc.Get(t.Context(), "tiny-ttl")
	require.ErrorIs(t, err, types.ErrEntryNotFound) // it must have expired

	// a sub-millisecond TTL cannot be honored and is rejected outright
	err = dc.Set(t.Context(), "sub-ms-ttl", "rejected", time.Nanosecond)
	require.ErrorIs(t, err, types.ErrTTLNotSupported)

	// a huge TTL
	err = dc.Set(t.Context(), "huge-ttl", "longterm", 100*365*24*time.Hour) // ~100 years
	require.NoError(t, err)
	val, err = dc.Get(t.Context(), "huge-ttl")
	require.NoError(t, err)
	require.Equal(t, "longterm", val)
}
