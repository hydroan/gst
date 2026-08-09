package cache_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/bigcache"
	"github.com/hydroan/gst/internal/cache/ccache"
	"github.com/hydroan/gst/internal/cache/cmap"
	"github.com/hydroan/gst/internal/cache/fastcache"
	"github.com/hydroan/gst/internal/cache/freecache"
	"github.com/hydroan/gst/internal/cache/lru"
	"github.com/hydroan/gst/internal/cache/lrue"
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

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{Redis: true})
}

func BenchmarkInt(b *testing.B) {
	b.Run("lru", func(b *testing.B) {
		benchInt(b, lru.Cache[int]())
	})
	b.Run("lrue", func(b *testing.B) {
		benchInt(b, lrue.Cache[int]())
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
	b.Run("lrue", func(b *testing.B) {
		benchIntParallel(b, lrue.Cache[int]())
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
	b.Run("lrue", func(b *testing.B) {
		benchString(b, lrue.Cache[string]())
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
	b.Run("lrue", func(b *testing.B) {
		benchStringParallel(b, lrue.Cache[string]())
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
	b.Run("lrue", func(b *testing.B) {
		benchUser(b, lrue.Cache[User]())
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
	b.Run("lrue", func(b *testing.B) {
		benchUserParallel(b, lrue.Cache[User]())
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
// the ttl contract, such as global-expiration backends.
func skipWhenSetUnsupported[T any](b *testing.B, cache types.Cache[T], probe T) {
	b.Helper()
	if err := cache.Set(context.Background(), "bench-probe", probe, 0); errors.Is(err, types.ErrTTLNotSupported) {
		b.Skip("backend cannot store entries under the ttl contract")
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
		for i := range benchKeyCount {
			_ = cache.Set(ctx, keys[i], values[i], 0)
		}
		b.ResetTimer()
		for i := 0; b.Loop(); i++ {
			_, _ = cache.Get(ctx, keys[i%benchKeyCount])
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
		for i := range benchKeyCount {
			_ = cache.Set(ctx, keys[i], values[i], 0)
		}
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			i := 0
			for p.Next() {
				_, _ = cache.Get(ctx, keys[i%benchKeyCount])
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
