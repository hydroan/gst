package dcache_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/dcache"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/types"
)

var ttl = 1 * time.Minute

func Benchmark(b *testing.B) {
	localcache, err := dcache.NewLocalCache[string]()
	if err != nil {
		b.Fatal(err)
	}
	redisCli, err := redis.New(config.App.Redis)
	if err != nil {
		b.Fatal(err)
	}
	redisCache, err := dcache.NewRedisCache(context.TODO(), redisCli, dcache.WithRedisKeyPrefix[string]("bench-"))
	if err != nil {
		b.Fatal(err)
	}
	redisCache2, err := dcache.NewRedisCache(context.TODO(), redisCli, dcache.WithRedisKeyPrefix[any]("any-bench-"))
	if err != nil {
		b.Fatal(err)
	}
	distributed, err := dcache.NewDistributedCache(
		dcache.WithMaxGoroutines[string](1000000),
		dcache.WithKafkaBrokers[string]([]string{"127.0.0.1:9092"}),
		dcache.WithRedisCache[string](redisCache2),
	)
	if err != nil {
		b.Fatal(err)
	}
	_ = redisCache
	_ = localcache

	b.Run("local", func(b *testing.B) {
		benchmark(b, localcache)
	})
	b.Run("redis", func(b *testing.B) {
		benchmark(b, redisCache)
	})
	b.Run("distributed", func(b *testing.B) {
		benchmark(b, distributed)
	})

	b.Run("local", func(b *testing.B) {
		benchmarkParallel(b, localcache)
	})
	b.Run("redis", func(b *testing.B) {
		benchmarkParallel(b, redisCache)
	})
	b.Run("distributed", func(b *testing.B) {
		benchmarkParallel(b, distributed)
	})

	// Output 2025-05-21 14:48
	//
	// goos: darwin
	// goarch: arm64
	// pkg: wcs/common/cache
	// cpu: Apple M4 Pro
	// Benchmark/local/set-14           1521928               777.8 ns/op
	// Benchmark/local/get-14          16304512                88.36 ns/op
	// Benchmark/local/mixed-14         2770002               436.0 ns/op
	// Benchmark/local/delete-14        6980763               182.3 ns/op
	// Benchmark/redis/set-14             13200             81401 ns/op
	// Benchmark/redis/get-14             12919             79628 ns/op
	// Benchmark/redis/mixed-14           13056             79058 ns/op
	// Benchmark/redis/delete-14          14548             80266 ns/op
	// Benchmark/distributed/set-14              619897              1796 ns/op
	// Benchmark/distributed/setwithsync-14      672477              1759 ns/op
	// Benchmark/distributed/get-14            10933824               115.2 ns/op
	// Benchmark/distributed/getwithsync-14    10717821               115.9 ns/op
	// Benchmark/distributed/mixed-14           1584810               740.3 ns/op
	// Benchmark/distributed/delete-14           938476              1233 ns/op
	// Benchmark/distributed/deletewithsync-14                   984937              1207 ns/op
	// Benchmark/local#01/parallel_set-14                        788176              1360 ns/op
	// Benchmark/local#01/parallel_get-14                      45268044                26.29 ns/op
	// Benchmark/local#01/parallel_mixed-14                     2346274               507.5 ns/op
	// Benchmark/redis#01/parallel_set-14                         53385             21954 ns/op
	// Benchmark/redis#01/parallel_get-14                         55778             22952 ns/op
	// Benchmark/redis#01/parallel_mixed-14                       55168             22419 ns/op
	// Benchmark/distributed#01/parallel_set-14                  499090              2388 ns/op
	// Benchmark/distributed#01/parallel_get-14                13284181                89.32 ns/op
	// Benchmark/distributed#01/parallel_mixed-14               1304516               914.3 ns/op
	// PASS
	// ok      wcs/common/cache        54.058s
}

func benchmark(b *testing.B, cache any) {
	b.Helper()
	count := 10000
	keys := make([]string, count)
	values := make([]string, count)
	cm := cache.(types.Cache[string]) //nolint:errcheck
	for i := range count {
		keys[i] = fmt.Sprintf("key-%d", i)
		values[i] = fmt.Sprintf("value-%d", i)
	}

	b.Run("set", func(b *testing.B) {
		for i := range b.N {
			idx := i % count
			if err := cm.Set(keys[idx], values[idx], ttl); err != nil {
				b.Fatal(err)
			}
		}
	})
	if dcm, ok := cache.(types.DistributedCache[string]); ok {
		b.Run("setwithsync", func(b *testing.B) {
			for i := range b.N {
				idx := i % count
				if err := dcm.SetWithSync(keys[idx], values[idx], ttl, ttl); err != nil {
					b.Fatal(err)
				}
			}
		})
	}

	b.Run("get", func(b *testing.B) {
		for i := range count {
			if err := cm.Set(keys[i], values[i], ttl); err != nil {
				b.Fatal(err)
			}
		}
		b.ResetTimer()

		for i := range b.N {
			idx := i % count
			if _, err := cm.Get(keys[idx]); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
				b.Fatal(err)
			}
		}
	})
	if dcm, ok := cache.(types.DistributedCache[string]); ok {
		b.Run("getwithsync", func(b *testing.B) {
			for i := range count {
				if err := dcm.Set(keys[i], values[i], ttl); err != nil {
					b.Fatal(err)
				}
			}
			b.ResetTimer()

			for i := range b.N {
				idx := i % count
				if _, err := dcm.GetWithSync(keys[idx], ttl); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
					b.Fatal(err)
				}
			}
		})
	}

	b.Run("mixed", func(b *testing.B) {
		for i := range count / 2 {
			if err := cm.Set(keys[i], values[i], ttl); err != nil {
				b.Fatal(err)
			}
		}
		b.ResetTimer()

		for i := range b.N {
			idx := i % count
			if i%3 == 0 {
				// 30% set
				if err := cm.Set(keys[idx], values[idx], ttl); err != nil {
					b.Fatal(err)
				}
			} else {
				// 70% get
				if _, err := cm.Get(keys[idx]); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
					b.Fatal(err)
				}
			}
		}
	})

	b.Run("delete", func(b *testing.B) {
		for i := range count {
			if err := cm.Set(keys[i], values[i], ttl); err != nil {
				b.Fatal(err)
			}
		}
		b.ResetTimer()

		for i := range b.N {
			idx := i % count
			if err := cm.Delete(keys[idx]); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
				b.Fatal(err)
			}
		}
	})
	if dcm, ok := cache.(types.DistributedCache[string]); ok {
		b.Run("deletewithsync", func(b *testing.B) {
			for i := range count {
				if err := dcm.SetWithSync(keys[i], values[i], ttl, ttl); err != nil {
					b.Fatal(err)
				}
			}
			b.ResetTimer()

			for i := range b.N {
				idx := i % count
				if err := dcm.DeleteWithSync(keys[idx]); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
					b.Fatal(err)
				}
			}
		})
	}
}

func benchmarkParallel(b *testing.B, cm types.Cache[string]) {
	b.Helper()
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
				err := cm.Set(keys[idx], values[idx], ttl)
				if err != nil {
					b.Fatal(err)
				}
				counter++
			}
		})
	})

	b.Run("parallel_get", func(b *testing.B) {
		for i := range count {
			err := cm.Set(keys[i], values[i], ttl)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			counter := 0
			for pb.Next() {
				idx := counter % count
				if _, err := cm.Get(keys[idx]); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
					b.Fatal(err)
				}
				counter++
			}
		})
	})

	b.Run("parallel_mixed", func(b *testing.B) {
		for i := range count / 2 {
			err := cm.Set(keys[i], values[i], ttl)
			if err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			counter := 0
			for pb.Next() {
				idx := counter % count

				// 使用质数哈希来分散操作类型，避免规律性
				opType := (counter * 7) % 10 // 将操作分为10类

				switch {
				case opType < 3: // 30% 写操作
					err := cm.Set(keys[idx], values[idx], ttl)
					if err != nil {
						b.Fatal(err)
					}
				case opType < 9: // 60% 读操作
					if _, err := cm.Get(keys[idx]); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
						b.Fatal(err)
					}
				default: // 10% 删除操作
					if err := cm.Delete(keys[idx]); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
						b.Fatal(err)
					}
				}

				counter++
			}
		})
	})
}

// func newRedis() redis.UniversalClient {
// 	opts := &redis.Options{Addr: redisAddr, Password: "password123", DB: 0}
// 	opts.PoolSize = redisPoolSize
// 	opts.MaxIdleConns = redisMaxIdleConns
//
// 	return redis.NewClient(opts)
// }
