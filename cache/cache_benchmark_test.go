package cache_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/cache"
	"github.com/hydroan/gst/internal/cache/bigcache"
	"github.com/hydroan/gst/internal/cache/ccache"
	"github.com/hydroan/gst/internal/cache/cmap"
	"github.com/hydroan/gst/internal/cache/fastcache"
	"github.com/hydroan/gst/internal/cache/freecache"
	"github.com/hydroan/gst/internal/cache/freelru"
	"github.com/hydroan/gst/internal/cache/lru"
	"github.com/hydroan/gst/internal/cache/otter"
	"github.com/hydroan/gst/internal/cache/ristretto"
	"github.com/hydroan/gst/internal/cache/smap"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/types"
)

type User struct {
	Name string `json:"name,omitempty"`
	model.Base
}

// Recorded results — Apple M4 Pro (14 cores), go1.26.4 darwin/arm64,
// 2026-08-09, struct values, 1000-key working set, redis on a local
// container. Reproduce with:
//
//	go test -run '^$' -bench 'BenchmarkUser' -benchtime 300ms ./cache/
//
// Mixed — 90% reads, 10% writes — comes first because it is the shape a
// cache actually sees; the single-operation columns follow as breakdown.
// Rows are grouped by what a value is stored as, because that decides which
// backends are candidates at all, and both tables carry the same row order,
// sorted by parallel Mixed, so a backend can be followed across them.
//
// Serial, ns/op:
//
//	backend    Mixed     Set     Get  Exists  Delete
//	-- live values --
//	smap        50.7    87.1    45.1    14.8     8.4
//	freelru     58.2    36.9    58.8    19.3    11.0
//	otter      190.6   391.9   170.7   110.5    29.3
//	cmap        55.9    30.4    58.1    13.2     9.6
//	ristretto  238.2   718.3   126.1    80.9   235.6
//	lru         54.2    38.0    53.8     9.8    13.3
//	ccache     295.2   450.1   225.5   222.0    27.9
//	-- serialized --
//	fastcache  353.1   341.7   331.7    31.0    13.5
//	freecache  369.1   350.3   359.3    92.9    13.1
//	bigcache   335.2   388.4   306.7    41.1    11.0
//	-- remote --
//	redis      91125   94295   91316   85901   86011
//
// Parallel across 14 cores, ns/op:
//
//	backend    Mixed     Set     Get
//	-- live values --
//	smap         8.4    30.1     3.7
//	freelru     27.2    27.8    26.7
//	otter       30.2   391.9    14.9
//	cmap        39.1    41.2    21.5
//	ristretto  167.2  1357.0    15.1
//	lru        221.5   199.4   219.7
//	ccache     385.1   796.2    38.5
//	-- serialized --
//	fastcache   42.7    59.0    39.6
//	freecache   71.0    74.3    62.3
//	bigcache   131.4   130.4    51.5
//	-- remote --
//	redis      21018   21146   20646
//
// How to read this. Under contention Mixed is the only column that separates
// backends whose write path blocks readers from those where the two are
// independent: ccache sits at 385 ns against freelru's 27 ns, a gap that
// neither the Set nor the Get column shows on its own, because ccache
// funnels every write through one worker goroutine. lru is flat across all
// three columns for the same reason, one global mutex.
//
// The tables hold the handle across the measured loop. Resolving it inline
// on every operation, which the facade invites by promising there is nothing
// to initialize, costs 66.3 ns against 51.1 ns serial and 184.3 against 157.0
// on 14 cores, allocating nothing either way — worth hoisting in a tight
// loop, not worth restructuring code for.
//
// Speed is not the selection criterion. cmap, smap and lru are fast because
// they never expire anything and mostly never evict; ristretto's fast reads
// come from the admission policy that silently drops most writes once the
// cache is warm; otter keeps writes visible but evicts new entries first; and
// the serialized backends pay an encoding round trip per operation while
// being unusable for any type whose state lives in unexported fields. See the
// cache package documentation for what actually decides the forwarded backend.

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{Redis: true})
}

func BenchmarkInt(b *testing.B) {
	b.Run("lru", func(b *testing.B) {
		benchInt(b, lru.Cache[int]())
	})
	b.Run("cmap", func(b *testing.B) {
		benchInt(b, cmap.Cache[int]())
	})
	b.Run("smap", func(b *testing.B) {
		benchInt(b, smap.Cache[int]())
	})
	b.Run("bigcache", func(b *testing.B) {
		benchInt(b, bigcache.Cache[int]())
	})
	b.Run("freecache", func(b *testing.B) {
		benchInt(b, freecache.Cache[int]())
	})
	b.Run("fastcache", func(b *testing.B) {
		benchInt(b, fastcache.Cache[int]())
	})
	b.Run("ccache", func(b *testing.B) {
		benchInt(b, ccache.Cache[int]())
	})
	b.Run("freelru", func(b *testing.B) {
		benchInt(b, freelru.Cache[int]())
	})
	b.Run("otter", func(b *testing.B) {
		benchInt(b, otter.Cache[int]())
	})
	b.Run("ristretto", func(b *testing.B) {
		benchInt(b, ristretto.Cache[int]())
	})
	b.Run("redis", func(b *testing.B) {
		benchInt(b, redis.Cache[int]())
	})
}

func BenchmarkIntParallel(b *testing.B) {
	b.Run("lru", func(b *testing.B) {
		benchIntParallel(b, lru.Cache[int]())
	})
	b.Run("cmap", func(b *testing.B) {
		benchIntParallel(b, cmap.Cache[int]())
	})
	b.Run("smap", func(b *testing.B) {
		benchIntParallel(b, smap.Cache[int]())
	})
	b.Run("bigcache", func(b *testing.B) {
		benchIntParallel(b, bigcache.Cache[int]())
	})
	b.Run("freecache", func(b *testing.B) {
		benchIntParallel(b, freecache.Cache[int]())
	})
	b.Run("fastcache", func(b *testing.B) {
		benchIntParallel(b, fastcache.Cache[int]())
	})
	b.Run("ccache", func(b *testing.B) {
		benchIntParallel(b, ccache.Cache[int]())
	})
	b.Run("freelru", func(b *testing.B) {
		benchIntParallel(b, freelru.Cache[int]())
	})
	b.Run("otter", func(b *testing.B) {
		benchIntParallel(b, otter.Cache[int]())
	})
	b.Run("ristretto", func(b *testing.B) {
		benchIntParallel(b, ristretto.Cache[int]())
	})
	b.Run("redis", func(b *testing.B) {
		benchIntParallel(b, redis.Cache[int]())
	})
}

func BenchmarkString(b *testing.B) {
	b.Run("lru", func(b *testing.B) {
		benchString(b, lru.Cache[string]())
	})
	b.Run("cmap", func(b *testing.B) {
		benchString(b, cmap.Cache[string]())
	})
	b.Run("smap", func(b *testing.B) {
		benchString(b, smap.Cache[string]())
	})
	b.Run("bigcache", func(b *testing.B) {
		benchString(b, bigcache.Cache[string]())
	})
	b.Run("freecache", func(b *testing.B) {
		benchString(b, freecache.Cache[string]())
	})
	b.Run("fastcache", func(b *testing.B) {
		benchString(b, fastcache.Cache[string]())
	})
	b.Run("ccache", func(b *testing.B) {
		benchString(b, ccache.Cache[string]())
	})
	b.Run("freelru", func(b *testing.B) {
		benchString(b, freelru.Cache[string]())
	})
	b.Run("otter", func(b *testing.B) {
		benchString(b, otter.Cache[string]())
	})
	b.Run("ristretto", func(b *testing.B) {
		benchString(b, ristretto.Cache[string]())
	})
	b.Run("redis", func(b *testing.B) {
		benchString(b, redis.Cache[string]())
	})
}

func BenchmarkStringParallel(b *testing.B) {
	b.Run("lru", func(b *testing.B) {
		benchStringParallel(b, lru.Cache[string]())
	})
	b.Run("cmap", func(b *testing.B) {
		benchStringParallel(b, cmap.Cache[string]())
	})
	b.Run("smap", func(b *testing.B) {
		benchStringParallel(b, smap.Cache[string]())
	})
	b.Run("bigcache", func(b *testing.B) {
		benchStringParallel(b, bigcache.Cache[string]())
	})
	b.Run("freecache", func(b *testing.B) {
		benchStringParallel(b, freecache.Cache[string]())
	})
	b.Run("fastcache", func(b *testing.B) {
		benchStringParallel(b, fastcache.Cache[string]())
	})
	b.Run("ccache", func(b *testing.B) {
		benchStringParallel(b, ccache.Cache[string]())
	})
	b.Run("freelru", func(b *testing.B) {
		benchStringParallel(b, freelru.Cache[string]())
	})
	b.Run("otter", func(b *testing.B) {
		benchStringParallel(b, otter.Cache[string]())
	})
	b.Run("ristretto", func(b *testing.B) {
		benchStringParallel(b, ristretto.Cache[string]())
	})
	b.Run("redis", func(b *testing.B) {
		benchStringParallel(b, redis.Cache[string]())
	})
}

func BenchmarkUser(b *testing.B) {
	b.Run("lru", func(b *testing.B) {
		benchUser(b, lru.Cache[User]())
	})
	b.Run("cmap", func(b *testing.B) {
		benchUser(b, cmap.Cache[User]())
	})
	b.Run("smap", func(b *testing.B) {
		benchUser(b, smap.Cache[User]())
	})
	b.Run("bigcache", func(b *testing.B) {
		benchUser(b, bigcache.Cache[User]())
	})
	b.Run("freecache", func(b *testing.B) {
		benchUser(b, freecache.Cache[User]())
	})
	b.Run("fastcache", func(b *testing.B) {
		benchUser(b, fastcache.Cache[User]())
	})
	b.Run("ccache", func(b *testing.B) {
		benchUser(b, ccache.Cache[User]())
	})
	b.Run("freelru", func(b *testing.B) {
		benchUser(b, freelru.Cache[User]())
	})
	b.Run("otter", func(b *testing.B) {
		benchUser(b, otter.Cache[User]())
	})
	b.Run("ristretto", func(b *testing.B) {
		benchUser(b, ristretto.Cache[User]())
	})
	b.Run("redis", func(b *testing.B) {
		benchUser(b, redis.Cache[User]())
	})
}

func BenchmarkUserParallel(b *testing.B) {
	b.Run("lru", func(b *testing.B) {
		benchUserParallel(b, lru.Cache[User]())
	})
	b.Run("cmap", func(b *testing.B) {
		benchUserParallel(b, cmap.Cache[User]())
	})
	b.Run("smap", func(b *testing.B) {
		benchUserParallel(b, smap.Cache[User]())
	})
	b.Run("bigcache", func(b *testing.B) {
		benchUserParallel(b, bigcache.Cache[User]())
	})
	b.Run("freecache", func(b *testing.B) {
		benchUserParallel(b, freecache.Cache[User]())
	})
	b.Run("fastcache", func(b *testing.B) {
		benchUserParallel(b, fastcache.Cache[User]())
	})
	b.Run("ccache", func(b *testing.B) {
		benchUserParallel(b, ccache.Cache[User]())
	})
	b.Run("freelru", func(b *testing.B) {
		benchUserParallel(b, freelru.Cache[User]())
	})
	b.Run("otter", func(b *testing.B) {
		benchUserParallel(b, otter.Cache[User]())
	})
	b.Run("ristretto", func(b *testing.B) {
		benchUserParallel(b, ristretto.Cache[User]())
	})
	b.Run("redis", func(b *testing.B) {
		benchUserParallel(b, redis.Cache[User]())
	})
}

// benchKeyCount bounds the working set well below the configured capacity so
// the Get benchmarks measure the hit path.
const benchKeyCount = 1000

// benchKeys precomputes the key working set so key construction stays out of
// the measured loops.
func benchKeys() []string {
	keys := make([]string, benchKeyCount)
	for i := range benchKeyCount {
		keys[i] = "key" + strconv.Itoa(i)
	}
	return keys
}

func benchIntValues() []int {
	values := make([]int, benchKeyCount)
	for i := range benchKeyCount {
		values[i] = i
	}
	return values
}

func benchStringValues() []string {
	values := make([]string, benchKeyCount)
	for i := range benchKeyCount {
		values[i] = strconv.Itoa(i)
	}
	return values
}

func benchUserValues() []User {
	values := make([]User, benchKeyCount)
	for i := range benchKeyCount {
		values[i] = User{Name: "user" + strconv.Itoa(i)}
	}
	return values
}

// skipWhenSetUnsupported skips backends whose Set cannot store entries under
// the ttl contract. Every benchmark here stores with ttl 0, the contract's
// "never expires", so a backend reaching this skip stores nothing at all —
// measuring its Get would only time misses. Only the global-expiration
// backends, whose Set rejects every lifetime by design, land here.
func skipWhenSetUnsupported[T any](b *testing.B, cache types.Cache[T], probe T) {
	b.Helper()
	if err := cache.Set(context.Background(), "bench-probe", probe, 0); errors.Is(err, types.ErrTTLNotSupported) {
		b.Skip("backend cannot store entries under the ttl contract")
	}
}

// fill primes the working set so read benchmarks measure the hit path.
func fill[T any](ctx context.Context, cache types.Cache[T], keys []string, values []T) {
	for i := range benchKeyCount {
		_ = cache.Set(ctx, keys[i], values[i], 0)
	}
}

func benchCache[T any](b *testing.B, cache types.Cache[T], values []T) {
	b.Helper()
	ctx := context.Background()
	keys := benchKeys()
	skipWhenSetUnsupported(b, cache, values[0])

	b.Run("Set", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			idx := i % benchKeyCount
			_ = cache.Set(ctx, keys[idx], values[idx], 0)
		}
	})
	b.Run("Get", func(b *testing.B) {
		fill(ctx, cache, keys, values)
		b.ResetTimer()
		for i := 0; b.Loop(); i++ {
			_, _ = cache.Get(ctx, keys[i%benchKeyCount])
		}
	})
	b.Run("Exists", func(b *testing.B) {
		fill(ctx, cache, keys, values)
		b.ResetTimer()
		for i := 0; b.Loop(); i++ {
			cache.Exists(ctx, keys[i%benchKeyCount])
		}
	})
	b.Run("Delete", func(b *testing.B) {
		fill(ctx, cache, keys, values)
		b.ResetTimer()
		for i := 0; b.Loop(); i++ {
			_ = cache.Delete(ctx, keys[i%benchKeyCount])
		}
	})
	// Mixed is the shape a cache actually sees: mostly hits, occasional
	// writes. The per-operation split is what separates backends whose write
	// path stalls readers from those where the two are independent.
	b.Run("Mixed", func(b *testing.B) {
		fill(ctx, cache, keys, values)
		b.ResetTimer()
		for i := 0; b.Loop(); i++ {
			idx := i % benchKeyCount
			if i%10 == 0 {
				_ = cache.Set(ctx, keys[idx], values[idx], 0)
				continue
			}
			_, _ = cache.Get(ctx, keys[idx])
		}
	})
}

func benchCacheParallel[T any](b *testing.B, cache types.Cache[T], values []T) {
	b.Helper()
	ctx := context.Background()
	keys := benchKeys()
	skipWhenSetUnsupported(b, cache, values[0])

	b.Run("Set Parallel", func(b *testing.B) {
		b.RunParallel(func(p *testing.PB) {
			i := 0
			for p.Next() {
				idx := i % benchKeyCount
				_ = cache.Set(ctx, keys[idx], values[idx], 0)
				i++
			}
		})
	})
	b.Run("Get Parallel", func(b *testing.B) {
		fill(ctx, cache, keys, values)
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			i := 0
			for p.Next() {
				_, _ = cache.Get(ctx, keys[i%benchKeyCount])
				i++
			}
		})
	})
	// Mixed under contention is where a backend's write path shows whether it
	// blocks readers: a single-worker write queue serializes every writer,
	// while a sharded or lock-free path does not.
	b.Run("Mixed Parallel", func(b *testing.B) {
		fill(ctx, cache, keys, values)
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			i := 0
			for p.Next() {
				idx := i % benchKeyCount
				if i%10 == 0 {
					_ = cache.Set(ctx, keys[idx], values[idx], 0)
				} else {
					_, _ = cache.Get(ctx, keys[idx])
				}
				i++
			}
		})
	})
}

func benchInt(b *testing.B, cache types.Cache[int]) {
	b.Helper()
	benchCache(b, cache, benchIntValues())
}

func benchIntParallel(b *testing.B, cache types.Cache[int]) {
	b.Helper()
	benchCacheParallel(b, cache, benchIntValues())
}

func benchString(b *testing.B, cache types.Cache[string]) {
	b.Helper()
	benchCache(b, cache, benchStringValues())
}

func benchStringParallel(b *testing.B, cache types.Cache[string]) {
	b.Helper()
	benchCacheParallel(b, cache, benchStringValues())
}

func benchUser(b *testing.B, cache types.Cache[User]) {
	b.Helper()
	benchCache(b, cache, benchUserValues())
}

func benchUserParallel(b *testing.B, cache types.Cache[User]) {
	b.Helper()
	benchCacheParallel(b, cache, benchUserValues())
}

// BenchmarkFacade measures the two ways a caller can reach the forwarded
// cache: resolving the handle on every operation, which the package
// documentation invites by promising there is nothing to initialize, and
// hoisting it once. The tables above measure the hoisted shape, so this is
// what says whether the inline shape costs anything worth avoiding.
func BenchmarkFacade(b *testing.B) {
	ctx := context.Background()
	_ = cache.Cache[User]().Set(ctx, "facade", User{Name: "n"}, 0)

	b.Run("Inline", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = cache.Cache[User]().Get(ctx, "facade")
		}
	})
	b.Run("Hoisted", func(b *testing.B) {
		c := cache.Cache[User]()
		b.ReportAllocs()
		for b.Loop() {
			_, _ = c.Get(ctx, "facade")
		}
	})
	b.Run("Inline Parallel", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				_, _ = cache.Cache[User]().Get(ctx, "facade")
			}
		})
	})
	b.Run("Hoisted Parallel", func(b *testing.B) {
		c := cache.Cache[User]()
		b.ReportAllocs()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				_, _ = c.Get(ctx, "facade")
			}
		})
	})
}
