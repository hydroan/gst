package multimap_test

import (
	"fmt"
	"testing"

	"github.com/hydroan/gst/ds/multimap"
)

func BenchmarkMultiMap_Get(b *testing.B) {
	b.Run("unsafe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for i := range b.N {
			mm.Get(fmt.Sprintf("key%d", i%100))
		}
	})

	b.Run("safe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for i := range b.N {
			mm.Get(fmt.Sprintf("key%d", i%100))
		}
	})
}

func BenchmarkMultiMap_GetOne(b *testing.B) {
	b.Run("unsafe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for i := range b.N {
			mm.GetOne(fmt.Sprintf("key%d", i%100))
		}
	})

	b.Run("safe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for i := range b.N {
			mm.GetOne(fmt.Sprintf("key%d", i%100))
		}
	})
}

func BenchmarkMultiMap_Set(b *testing.B) {
	b.Run("unsafe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		b.ResetTimer()

		for i := range b.N {
			mm.Set(fmt.Sprintf("key%d", i%100), i)
		}
	})

	b.Run("safe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		b.ResetTimer()

		for i := range b.N {
			mm.Set(fmt.Sprintf("key%d", i%100), i)
		}
	})
}

func BenchmarkMultiMap_SetAll(b *testing.B) {
	values := make([]int, 100)
	for i := range values {
		values[i] = i
	}

	b.Run("unsafe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		b.ResetTimer()
		for i := range b.N {
			mm.SetAll(fmt.Sprintf("key%d", i%100), values)
		}
	})

	b.Run("safe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		b.ResetTimer()
		for i := range b.N {
			mm.SetAll(fmt.Sprintf("key%d", i%100), values)
		}
	})
}

func BenchmarkMultiMap_Delete(b *testing.B) {
	b.Run("unsafe_exists", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		keys := make([]string, 100)
		for i := range 100 {
			key := fmt.Sprintf("key%d", i)
			keys[i] = key
			for j := range 5 {
				mm.Set(key, j)
			}
		}

		b.ResetTimer()
		for i := range b.N {
			mm.Delete(keys[i%100])
		}
	})

	b.Run("unsafe_not_exists", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		b.ResetTimer()
		for i := range b.N {
			mm.Delete(fmt.Sprintf("not_exists_key%d", i))
		}
	})

	b.Run("safe_exists", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		keys := make([]string, 100)
		for i := range 100 {
			key := fmt.Sprintf("key%d", i)
			keys[i] = key
			for j := range 5 {
				mm.Set(key, j)
			}
		}

		b.ResetTimer()
		for i := range b.N {
			mm.Delete(keys[i%100])
		}
	})

	b.Run("safe_not_exists", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		b.ResetTimer()
		for i := range b.N {
			mm.Delete(fmt.Sprintf("not_exists_key%d", i))
		}
	})
}

func BenchmarkMultiMap_DeleteValue(b *testing.B) {
	b.Run("unsafe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		key := "test_key"
		for i := range 100 {
			mm.Set(key, i)
		}

		b.ResetTimer()
		for i := range b.N {
			mm.DeleteValue(key, i%100)
		}
	})

	b.Run("safe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		key := "test_key"
		for i := range 100 {
			mm.Set(key, i)
		}

		b.ResetTimer()
		for i := range b.N {
			mm.DeleteValue(key, i%100)
		}
	})
}

func BenchmarkMultiMap_Has(b *testing.B) {
	b.Run("unsafe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for i := range b.N {
			mm.Has(fmt.Sprintf("key%d", i%100))
		}
	})

	b.Run("safe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for i := range b.N {
			mm.Has(fmt.Sprintf("key%d", i%100))
		}
	})
}

func BenchmarkMultiMap_Count(b *testing.B) {
	b.Run("unsafe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		key := "test_key"
		for i := range 100 {
			mm.Set(key, i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Count(key)
		}
	})

	b.Run("safe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		key := "test_key"
		for i := range 100 {
			mm.Set(key, i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Count(key)
		}
	})
}

func BenchmarkMultiMap_Contains(b *testing.B) {
	b.Run("unsafe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		for i := range 100 {
			mm.Set("key", i)
		}

		b.ResetTimer()
		for i := range b.N {
			mm.Contains("key", i%100)
		}
	})

	b.Run("safe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for i := range 100 {
			mm.Set("key", i)
		}

		b.ResetTimer()
		for i := range b.N {
			mm.Contains("key", i%100)
		}
	})
}

func BenchmarkMultiMap_Size(b *testing.B) {
	b.Run("unsafe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Size()
		}
	})

	b.Run("safe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Size()
		}
	})
}

func BenchmarkMultiMap_IsEmpty(b *testing.B) {
	b.Run("unsafe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.IsEmpty()
		}
	})

	b.Run("safe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.IsEmpty()
		}
	})
}

func BenchmarkMultiMap_Keys(b *testing.B) {
	b.Run("unsafe_small", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Keys()
		}
	})

	b.Run("unsafe_large", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		for i := range 10000 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Keys()
		}
	})

	b.Run("safe_small", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Keys()
		}
	})

	b.Run("safe_large", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for i := range 10000 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Keys()
		}
	})
}

func BenchmarkMultiMap_Values(b *testing.B) {
	b.Run("unsafe_small", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		for i := range 100 {
			for j := range 5 { // 每个key存5个值
				mm.Set(fmt.Sprintf("key%d", i), i*10+j)
			}
		}

		b.ResetTimer()
		for range b.N {
			mm.Values()
		}
	})

	b.Run("unsafe_large", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		for i := range 1000 {
			for j := range 10 { // 每个key存10个值
				mm.Set(fmt.Sprintf("key%d", i), i*10+j)
			}
		}

		b.ResetTimer()
		for range b.N {
			mm.Values()
		}
	})

	b.Run("safe_small", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for i := range 100 {
			for j := range 5 {
				mm.Set(fmt.Sprintf("key%d", i), i*10+j)
			}
		}

		b.ResetTimer()
		for range b.N {
			mm.Values()
		}
	})

	b.Run("safe_large", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for i := range 1000 {
			for j := range 10 {
				mm.Set(fmt.Sprintf("key%d", i), i*10+j)
			}
		}

		b.ResetTimer()
		for range b.N {
			mm.Values()
		}
	})
}

func BenchmarkMultiMap_Clear(b *testing.B) {
	b.Run("unsafe_small", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		// 预填充一次，避免在循环中重复填充
		for j := range 100 {
			for k := range 5 {
				mm.Set(fmt.Sprintf("key%d", j), k)
			}
		}

		b.ResetTimer()
		for range b.N {
			// 使用 Clone 来获取新的副本进行清理
			clone := mm.Clone()
			clone.Clear()
		}
	})

	b.Run("unsafe_medium", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		// 使用中等规模的数据：500个键，每个键5个值
		for j := range 500 {
			for k := range 5 {
				mm.Set(fmt.Sprintf("key%d", j), k)
			}
		}

		b.ResetTimer()
		for range b.N {
			clone := mm.Clone()
			clone.Clear()
		}
	})

	b.Run("safe_small", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for j := range 100 {
			for k := range 5 {
				mm.Set(fmt.Sprintf("key%d", j), k)
			}
		}

		b.ResetTimer()
		for range b.N {
			clone := mm.Clone()
			clone.Clear()
		}
	})

	b.Run("safe_medium", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for j := range 500 {
			for k := range 5 {
				mm.Set(fmt.Sprintf("key%d", j), k)
			}
		}

		b.ResetTimer()
		for range b.N {
			clone := mm.Clone()
			clone.Clear()
		}
	})
}

func BenchmarkMultiMap_Clone(b *testing.B) {
	b.Run("unsafe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		for i := range 1000 {
			mm.Set(fmt.Sprintf("key%d", i%100), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Clone()
		}
	})

	b.Run("safe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for i := range 1000 {
			mm.Set(fmt.Sprintf("key%d", i%100), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Clone()
		}
	})
}

func BenchmarkMultiMap_Map(b *testing.B) {
	b.Run("unsafe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Map()
		}
	})

	b.Run("safe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Map()
		}
	})
}

func BenchmarkMultiMap_Range(b *testing.B) {
	b.Run("unsafe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp)
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Range(func(key string, values []int) bool {
				return true
			})
		}
	})

	b.Run("safe", func(b *testing.B) {
		mm, _ := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
		for i := range 100 {
			mm.Set(fmt.Sprintf("key%d", i), i)
		}

		b.ResetTimer()
		for range b.N {
			mm.Range(func(key string, values []int) bool {
				return true
			})
		}
	})
}
