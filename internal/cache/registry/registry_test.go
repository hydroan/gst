package registry_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hydroan/gst/internal/cache/registry"
)

// TestLoadCreatesOncePerType asserts the double-checked creation: concurrent
// first loads of one type run create exactly once and share the instance.
func TestLoadCreatesOncePerType(t *testing.T) {
	store := registry.New()
	var created atomic.Int32

	const workers = 8
	results := make([]*int, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = registry.Load(store, func() *int {
				created.Add(1)
				return new(int)
			})
		}(i)
	}
	wg.Wait()

	if got := created.Load(); got != 1 {
		t.Fatalf("want create to run once, ran %d times", got)
	}
	for i := 1; i < workers; i++ {
		if results[i] != results[0] {
			t.Fatal("want every load to return the same instance")
		}
	}
}

// TestLoadKeysByType asserts that distinct types get distinct instances.
func TestLoadKeysByType(t *testing.T) {
	store := registry.New()

	intInstance := registry.Load(store, func() *int { return new(int) })
	stringInstance := registry.Load(store, func() *string { return new(string) })

	if registry.Load(store, func() *int { t.Fatal("create must not rerun"); return nil }) != intInstance {
		t.Fatal("want the registered int instance")
	}
	if registry.Load(store, func() *string { t.Fatal("create must not rerun"); return nil }) != stringInstance {
		t.Fatal("want the registered string instance")
	}
}
