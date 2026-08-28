package dcache_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/cache"
	"github.com/hydroan/gst/dcache"
	"github.com/hydroan/gst/types"
)

var ttl = 1 * time.Minute

func Benchmark(b *testing.B) {
	localcache := cache.Cache[string]()
	replicated, err := dcache.Cache[string]()
	if err != nil {
		b.Fatal(err)
	}

	b.Run("local", func(b *testing.B) {
		benchmark(b, localcache)
	})
	b.Run("replicated", func(b *testing.B) {
		benchmark(b, replicated)
	})

	b.Run("local_parallel", func(b *testing.B) {
		benchmarkParallel(b, localcache)
	})
	b.Run("replicated_parallel", func(b *testing.B) {
		benchmarkParallel(b, replicated)
	})
}

func benchmark(b *testing.B, cm types.Cache[string]) {
	b.Helper()
	ctx := context.Background()
	count := 10000
	keys := make([]string, count)
	values := make([]string, count)
	for i := range count {
		keys[i] = fmt.Sprintf("key-%d", i)
		values[i] = fmt.Sprintf("value-%d", i)
	}

	b.Run("set", func(b *testing.B) {
		for i := range b.N {
			idx := i % count
			if err := cm.Set(ctx, keys[idx], values[idx], ttl); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("get", func(b *testing.B) {
		for i := range count {
			if err := cm.Set(ctx, keys[i], values[i], ttl); err != nil {
				b.Fatal(err)
			}
		}
		b.ResetTimer()

		for i := range b.N {
			idx := i % count
			if _, err := cm.Get(ctx, keys[idx]); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
				b.Fatal(err)
			}
		}
	})

	b.Run("mixed", func(b *testing.B) {
		for i := range count / 2 {
			if err := cm.Set(ctx, keys[i], values[i], ttl); err != nil {
				b.Fatal(err)
			}
		}
		b.ResetTimer()

		for i := range b.N {
			idx := i % count
			if i%3 == 0 {
				// 30% set
				if err := cm.Set(ctx, keys[idx], values[idx], ttl); err != nil {
					b.Fatal(err)
				}
			} else {
				// 70% get
				if _, err := cm.Get(ctx, keys[idx]); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
					b.Fatal(err)
				}
			}
		}
	})

	b.Run("delete", func(b *testing.B) {
		for i := range count {
			if err := cm.Set(ctx, keys[i], values[i], ttl); err != nil {
				b.Fatal(err)
			}
		}
		b.ResetTimer()

		for i := range b.N {
			idx := i % count
			if err := cm.Delete(ctx, keys[idx]); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
				b.Fatal(err)
			}
		}
	})
}

func benchmarkParallel(b *testing.B, cm types.Cache[string]) {
	b.Helper()
	ctx := context.Background()
	count := 10000
	keys := make([]string, count)
	values := make([]string, count)
	for i := range count {
		keys[i] = fmt.Sprintf("parallel-key-%d", i)
		values[i] = fmt.Sprintf("parallel-value-%d", i)
	}

	b.Run("parallel_set", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			counter := 0
			for pb.Next() {
				idx := counter % count
				err := cm.Set(ctx, keys[idx], values[idx], ttl)
				if err != nil {
					b.Fatal(err)
				}
				counter++
			}
		})
	})

	b.Run("parallel_get", func(b *testing.B) {
		for i := range count {
			err := cm.Set(ctx, keys[i], values[i], ttl)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			counter := 0
			for pb.Next() {
				idx := counter % count
				if _, err := cm.Get(ctx, keys[idx]); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
					b.Fatal(err)
				}
				counter++
			}
		})
	})

	b.Run("parallel_mixed", func(b *testing.B) {
		for i := range count / 2 {
			err := cm.Set(ctx, keys[i], values[i], ttl)
			if err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			counter := 0
			for pb.Next() {
				idx := counter % count

				// hash with a prime to spread the operation types and avoid a regular pattern
				opType := (counter * 7) % 10 // split the operations into 10 classes

				switch {
				case opType < 3: // 30% writes
					err := cm.Set(ctx, keys[idx], values[idx], ttl)
					if err != nil {
						b.Fatal(err)
					}
				case opType < 9: // 60% reads
					if _, err := cm.Get(ctx, keys[idx]); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
						b.Fatal(err)
					}
				default: // 10% deletes
					if err := cm.Delete(ctx, keys[idx]); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
						b.Fatal(err)
					}
				}

				counter++
			}
		})
	})
}
