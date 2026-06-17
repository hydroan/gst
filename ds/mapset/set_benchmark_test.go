package mapset_test

import (
	"fmt"
	"testing"

	"github.com/hydroan/gst/ds/mapset"
)

func createSet(b *testing.B, size int, safe bool) *mapset.Set[int] {
	b.Helper()
	var set *mapset.Set[int]
	var err error
	if safe {
		set, err = mapset.New(mapset.WithSafe[int]())
	} else {
		set, err = mapset.New[int]()
	}
	if err != nil {
		b.Fatalf("failed to create mapset: %v", err)
	}
	for i := range size {
		set.Add(i)
	}
	return set
}

func benchmark(b *testing.B, hasConcUnsafe bool, sizes []int, do func(set *mapset.Set[int])) {
	b.Helper()
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			b.Run("single unsafe", func(b *testing.B) {
				set := createSet(b, size, false)
				b.ResetTimer()
				for range b.N {
					do(set)
				}
			})
			b.Run("single safe", func(b *testing.B) {
				set := createSet(b, size, true)
				b.ResetTimer()
				for range b.N {
					do(set)
				}
			})

			if hasConcUnsafe {
				b.Run("conc unsafe", func(b *testing.B) {
					set := createSet(b, size, false)
					b.ResetTimer()
					b.RunParallel(func(p *testing.PB) {
						for p.Next() {
							do(set)
						}
					})
				})
			}
			b.Run("conc safe", func(b *testing.B) {
				set := createSet(b, size, true)
				b.ResetTimer()
				b.RunParallel(func(p *testing.PB) {
					for p.Next() {
						do(set)
					}
				})
			})
		})
	}
}

func BenchmarkMapSet_Add(b *testing.B) {
	benchmark(b, false, []int{10, 100, 1000, 10000}, func(set *mapset.Set[int]) {
		set.Add(0)
	})
}

func BenchmarkMapSet_Pop(b *testing.B) {
	benchmark(b, false, []int{10, 100, 1000}, func(set *mapset.Set[int]) {
		_, _ = set.Pop()
	})
}

func BenchmarkMapSet_Remove(b *testing.B) {
	benchmark(b, false, []int{10, 100, 1000}, func(set *mapset.Set[int]) {
		set.Remove(0)
	})
}

func BenchmarkMapSet_Clear(b *testing.B) {
	benchmark(b, false, []int{10, 10000}, func(set *mapset.Set[int]) {
		set.Clear()
	})
}

func BenchmarkMapSet_Len(b *testing.B) {
	benchmark(b, true, []int{10, 100}, func(set *mapset.Set[int]) {
		_ = set.Len()
	})
}

func BenchmarkMapSet_Clone(b *testing.B) {
	benchmark(b, true, []int{10, 100}, func(set *mapset.Set[int]) {
		_ = set.Clone()
	})
}

func BenchmarkMapSet_Contains(b *testing.B) {
	benchmark(b, true, []int{10, 100}, func(set *mapset.Set[int]) {
		_ = set.Contains(-1, -2)
	})
}

func BenchmarkMapSet_ContainsOne(b *testing.B) {
	benchmark(b, true, []int{10, 100}, func(set *mapset.Set[int]) {
		_ = set.Contains(-1)
	})
}

func BenchmarkMapSet_ContainsAny(b *testing.B) {
	benchmark(b, true, []int{10, 100}, func(set *mapset.Set[int]) {
		_ = set.ContainsAny(-1, -2)
	})
}

func BenchmarkMapSet_ContainsAnyElement(b *testing.B) {
	for _, size := range []int{10, 100, 1000, 10000} {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			benchmarkContainsAnyElement(b, size)
		})
	}
}

func benchmarkContainsAnyElement(b *testing.B, size int) {
	b.Helper()
	b.Run("single unset", func(b *testing.B) {
		set := createSet(b, size, false)
		other := createSet(b, size, false)
		b.ResetTimer()
		for range b.N {
			_ = set.ContainsAnyElement(other)
		}
	})
	b.Run("single safe", func(b *testing.B) {
		set := createSet(b, size, true)
		other := createSet(b, size, true)
		b.ResetTimer()
		for range b.N {
			_ = set.ContainsAnyElement(other)
		}
	})

	b.Run("conc unsafe", func(b *testing.B) {
		set := createSet(b, size, false)
		other := createSet(b, size, false)
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				_ = set.ContainsAnyElement(other)
			}
		})
	})
	b.Run("conc safe", func(b *testing.B) {
		set := createSet(b, size, true)
		other := createSet(b, size, true)
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				_ = set.ContainsAnyElement(other)
			}
		})
	})
}

func BenchmarkMapSet_Range(b *testing.B) {
	benchmark(b, true, []int{10, 100}, func(set *mapset.Set[int]) {
		set.Range(func(e int) bool {
			_ = e
			return true
		})
	})
}

func BenchmarkMapSet_Equal(b *testing.B) {
	for _, size := range []int{10} {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			benchmarkEqual(b, size)
		})
	}
}

func benchmarkEqual(b *testing.B, size int) {
	b.Helper()
	b.Run("single unset", func(b *testing.B) {
		set := createSet(b, size, false)
		other := createSet(b, size, false)
		b.ResetTimer()
		for range b.N {
			_ = set.Equal(other)
		}
	})
	b.Run("single safe", func(b *testing.B) {
		set := createSet(b, size, true)
		other := createSet(b, size, true)
		b.ResetTimer()
		for range b.N {
			_ = set.Equal(other)
		}
	})

	b.Run("conc unsafe", func(b *testing.B) {
		set := createSet(b, size, false)
		other := createSet(b, size, false)
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				_ = set.Equal(other)
			}
		})
	})
	b.Run("conc safe", func(b *testing.B) {
		set := createSet(b, size, true)
		other := createSet(b, size, true)
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				_ = set.Equal(other)
			}
		})
	})
}

func BenchmarkMapSet_IsEmpty(b *testing.B) {
	benchmark(b, true, []int{10}, func(set *mapset.Set[int]) {
		_ = set.IsEmpty()
	})
}

func BenchmarkMapSet_Iter(b *testing.B) {
	benchmark(b, true, []int{10, 100}, func(set *mapset.Set[int]) {
		for e := range set.Iter() {
			_ = e
		}
	})
}

func BenchmarkMapSet_IsSubset(b *testing.B) {
	sup := createSet(b, 1000, true)
	benchmark(b, true, []int{10, 100}, func(set *mapset.Set[int]) {
		set.IsSubset(sup)
	})
}

func BenchmarkMapSet_IsProperSubset(b *testing.B) {
	sup := createSet(b, 1000, true)
	benchmark(b, true, []int{10, 100}, func(set *mapset.Set[int]) {
		set.IsProperSubset(sup)
	})
}

func BenchmarkMapSet_IsSuperSet(b *testing.B) {
	sub := createSet(b, 10, true)
	benchmark(b, true, []int{100, 1000}, func(set *mapset.Set[int]) {
		set.IsSuperset(sub)
	})
}

func BenchmarkMapSet_IsProperSuperset(b *testing.B) {
	sub := createSet(b, 10, true)
	benchmark(b, true, []int{100, 1000}, func(set *mapset.Set[int]) {
		set.IsProperSuperset(sub)
	})
}

func BenchmarkMapSet_Difference(b *testing.B) {
	other := createSet(b, 50, true)
	benchmark(b, true, []int{100, 1000}, func(set *mapset.Set[int]) {
		set.Difference(other)
	})
}

func BenchmarkMapSet_SymmetricDifference(b *testing.B) {
	other := createSet(b, 50, true)
	benchmark(b, true, []int{100, 1000}, func(set *mapset.Set[int]) {
		set.SymmetricDifference(other)
	})
}

func BenchmarkMapSet_Union(b *testing.B) {
	other := createSet(b, 100, true)
	benchmark(b, true, []int{100, 1000}, func(set *mapset.Set[int]) {
		set.Union(other)
	})
}

func BenchmarkMapSet_Intersect(b *testing.B) {
	other := createSet(b, 100, true)
	benchmark(b, true, []int{100, 1000}, func(set *mapset.Set[int]) {
		set.Intersect(other)
	})
}

func BenchmarkMapSet_String(b *testing.B) {
	benchmark(b, true, []int{100, 1000}, func(set *mapset.Set[int]) {
		_ = set.String()
	})
}

func BenchmarkMapSet_Slice(b *testing.B) {
	benchmark(b, true, []int{100, 1000}, func(set *mapset.Set[int]) {
		_ = set.Slice()
	})
}

func BenchmarkMapSet_MarshalJSON(b *testing.B) {
	benchmark(b, true, []int{100, 1000}, func(set *mapset.Set[int]) {
		_, _ = set.MarshalJSON()
	})
}

func BenchmarkMapSet_UnmarshalJSON(b *testing.B) {
	data := []byte("[1,2,3]")
	benchmark(b, false, []int{100, 1000}, func(set *mapset.Set[int]) {
		_ = set.UnmarshalJSON(data)
	})
}
