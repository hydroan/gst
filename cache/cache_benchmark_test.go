package cache_test

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hydroan/gst/cache"
	"github.com/hydroan/gst/cache/bigcache"
	"github.com/hydroan/gst/cache/ccache"
	"github.com/hydroan/gst/cache/cmap"
	"github.com/hydroan/gst/cache/fastcache"
	"github.com/hydroan/gst/cache/freecache"
	"github.com/hydroan/gst/cache/gocache"
	"github.com/hydroan/gst/cache/lru"
	"github.com/hydroan/gst/cache/lrue"
	"github.com/hydroan/gst/cache/ristretto"
	"github.com/hydroan/gst/cache/smap"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/provider/memcached"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/types"
)

type User struct {
	Name string `json:"name,omitempty"`
	model.Base
}

func init() {
	os.Setenv(config.REDIS_ENABLE, "true")
	os.Setenv(config.MEMCACHED_ENABLE, "false")
	os.Setenv(config.REDIS_ADDR, "127.0.0.1:6379")
	os.Setenv(config.REDIS_PASSWORD, "password123")
	if err := config.Init(); err != nil {
		panic(err)
	}
	if err := zap.Init(); err != nil {
		panic(err)
	}
	if err := redis.Init(); err != nil {
		panic(err)
	}
	if err := memcached.Init(); err != nil {
		panic(err)
	}
	if err := cache.Init(); err != nil {
		panic(err)
	}
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
	b.Run("gocache", func(b *testing.B) {
		benchInt(b, gocache.Cache[int]())
	})
	b.Run("ristretto", func(b *testing.B) {
		benchInt(b, ristretto.Cache[int]())
	})
	b.Run("redis", func(b *testing.B) {
		benchInt(b, redis.Cache[int]())
	})
	b.Run("memcached", func(b *testing.B) {
		benchInt(b, memcached.Cache[int]())
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
	b.Run("gocache", func(b *testing.B) {
		benchIntParallel(b, gocache.Cache[int]())
	})
	b.Run("ristretto", func(b *testing.B) {
		benchIntParallel(b, ristretto.Cache[int]())
	})
	b.Run("redis", func(b *testing.B) {
		benchIntParallel(b, redis.Cache[int]())
	})
	b.Run("memcached", func(b *testing.B) {
		benchIntParallel(b, memcached.Cache[int]())
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
	b.Run("gocache", func(b *testing.B) {
		benchString(b, gocache.Cache[string]())
	})
	b.Run("ristretto", func(b *testing.B) {
		benchString(b, ristretto.Cache[string]())
	})
	b.Run("redis", func(b *testing.B) {
		benchString(b, redis.Cache[string]())
	})
	b.Run("memcached", func(b *testing.B) {
		benchString(b, memcached.Cache[string]())
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
	b.Run("gocache", func(b *testing.B) {
		benchStringParallel(b, gocache.Cache[string]())
	})
	b.Run("ristretto", func(b *testing.B) {
		benchStringParallel(b, ristretto.Cache[string]())
	})
	b.Run("redis", func(b *testing.B) {
		benchStringParallel(b, redis.Cache[string]())
	})
	b.Run("memcached", func(b *testing.B) {
		benchStringParallel(b, memcached.Cache[string]())
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
	b.Run("gocache", func(b *testing.B) {
		benchUser(b, gocache.Cache[User]())
	})
	b.Run("ristretto", func(b *testing.B) {
		benchUser(b, ristretto.Cache[User]())
	})
	b.Run("redis", func(b *testing.B) {
		benchUser(b, redis.Cache[User]())
	})
	b.Run("memcached", func(b *testing.B) {
		benchUser(b, memcached.Cache[User]())
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
	b.Run("gocache", func(b *testing.B) {
		benchUserParallel(b, gocache.Cache[User]())
	})
	b.Run("ristretto", func(b *testing.B) {
		benchUserParallel(b, ristretto.Cache[User]())
	})
	b.Run("redis", func(b *testing.B) {
		benchUserParallel(b, redis.Cache[User]())
	})
	b.Run("memcached", func(b *testing.B) {
		benchUserParallel(b, memcached.Cache[User]())
	})
}

func benchInt(b *testing.B, cache types.Cache[int]) {
	b.Helper()
	b.Run("Set", func(b *testing.B) {
		for i := range b.N {
			_ = cache.Set(fmt.Sprintf("key%d", i), i, 0)
		}
	})
	b.Run("Get", func(b *testing.B) {
		for i := range b.N {
			_, _ = cache.Get(fmt.Sprintf("key%d", i))
		}
	})
}

func benchIntParallel(b *testing.B, cache types.Cache[int]) {
	b.Helper()
	b.Run("Set Parallel", func(b *testing.B) {
		b.RunParallel(func(p *testing.PB) {
			i := 0
			for p.Next() {
				_ = cache.Set(fmt.Sprintf("key%d", i), i, 0)
				i++
			}
		})
	})
	b.Run("Get Parallel", func(b *testing.B) {
		b.RunParallel(func(p *testing.PB) {
			i := 0
			for p.Next() {
				_, _ = cache.Get(fmt.Sprintf("key%d", i))
				i++
			}
		})
	})
}

func benchString(b *testing.B, cache types.Cache[string]) {
	b.Helper()
	b.Run("Set", func(b *testing.B) {
		for i := range b.N {
			_ = cache.Set(fmt.Sprintf("key%d", i), strconv.Itoa(i), 0)
		}
	})
	b.Run("Get", func(b *testing.B) {
		for i := range b.N {
			_, _ = cache.Get(fmt.Sprintf("key%d", i))
		}
	})
}

func benchStringParallel(b *testing.B, cache types.Cache[string]) {
	b.Helper()
	b.Run("Set Parallel", func(b *testing.B) {
		b.RunParallel(func(p *testing.PB) {
			i := 0
			for p.Next() {
				_ = cache.Set(fmt.Sprintf("key%d", i), strconv.Itoa(i), 0)
				i++
			}
		})
	})
	b.Run("Get Parallel", func(b *testing.B) {
		b.RunParallel(func(p *testing.PB) {
			i := 0
			for p.Next() {
				_, _ = cache.Get(fmt.Sprintf("key%d", i))
				i++
			}
		})
	})
}

func benchUser(b *testing.B, cache types.Cache[User]) {
	b.Helper()
	b.Run("Set", func(b *testing.B) {
		for i := range b.N {
			_ = cache.Set(fmt.Sprintf("key%d", i), User{Name: fmt.Sprintf("user%d", i)}, 0)
		}
	})
	b.Run("Get", func(b *testing.B) {
		for i := range b.N {
			_, _ = cache.Get(fmt.Sprintf("key%d", i))
		}
	})
}

func benchUserParallel(b *testing.B, cache types.Cache[User]) {
	b.Helper()
	b.Run("Set Parallel", func(b *testing.B) {
		b.RunParallel(func(p *testing.PB) {
			i := 0
			for p.Next() {
				_ = cache.Set(fmt.Sprintf("key%d", i), User{Name: fmt.Sprintf("user%d", i)}, 0)
				i++
			}
		})
	})
	b.Run("Get Parallel", func(b *testing.B) {
		b.RunParallel(func(p *testing.PB) {
			i := 0
			for p.Next() {
				_, _ = cache.Get(fmt.Sprintf("key%d", i))
				i++
			}
		})
	})
}
