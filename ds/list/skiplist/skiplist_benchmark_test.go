package skiplist_test

import (
	"fmt"
	"testing"

	"github.com/hydroan/gst/ds/list/skiplist"
)

func createSkipList(b *testing.B, size int, safe bool) *skiplist.SkipList[int64, int64] {
	b.Helper()
	var sl *skiplist.SkipList[int64, int64]
	var err error
	if safe {
		// sl, err = skiplist.NewOrdered(skiplist.WithSafe[int64, int64](), skiplist.WithProbability[int64, int64](0.33))
		sl, err = skiplist.NewOrdered(skiplist.WithSafe[int64, int64]())
	} else {
		// sl, err = skiplist.NewOrdered[int64, int64](skiplist.WithProbability[int64, int64](0.33))
		sl, err = skiplist.NewOrdered[int64, int64]()
	}
	if err != nil {
		b.Fatalf("failed to create skip list: %v", err)
	}
	for i := range size {
		sl.Put(int64(i), int64(i))
	}
	return sl
}

func benchmark(b *testing.B, sizes []int, do func(sl *skiplist.SkipList[int64, int64])) {
	b.Helper()
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			b.Run("single unsafe", func(b *testing.B) {
				sl := createSkipList(b, size, false)
				b.ResetTimer()
				for range b.N {
					do(sl)
				}
			})

			b.Run("single safe", func(b *testing.B) {
				sl := createSkipList(b, size, true)
				b.ResetTimer()
				for range b.N {
					do(sl)
				}
			})

			b.Run("concur safe", func(b *testing.B) {
				sl := createSkipList(b, size, true)
				b.ResetTimer()
				b.RunParallel(func(p *testing.PB) {
					for p.Next() {
						do(sl)
					}
				})
			})
		})
	}
}

func benchmarkIndex(b *testing.B, sizes []int, do func(sl *skiplist.SkipList[int64, int64], i int)) {
	b.Helper()
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			b.Run("single unsafe", func(b *testing.B) {
				sl := createSkipList(b, size, false)
				b.ResetTimer()
				for i := range b.N {
					do(sl, i)
				}
			})
			b.Run("single safe", func(b *testing.B) {
				sl := createSkipList(b, size, true)
				b.ResetTimer()
				for i := range b.N {
					do(sl, i)
				}
			})
			b.Run("concur safe", func(b *testing.B) {
				sl := createSkipList(b, size, true)
				b.ResetTimer()
				b.RunParallel(func(p *testing.PB) {
					i := 0
					for p.Next() {
						do(sl, i)
						i++
					}
				})
			})
		})
	}
}

func BenchmarkSkipList_Put(b *testing.B) {
	benchmarkIndex(b, []int{100}, func(sl *skiplist.SkipList[int64, int64], i int) {
		sl.Put(int64(i%100), int64(i%100))
	})
	benchmarkIndex(b, []int{1000000}, func(sl *skiplist.SkipList[int64, int64], i int) {
		sl.Put(int64(i%1000000), int64(i%1000000))
	})
}

func BenchmarkSkipList_Get(b *testing.B) {
	benchmarkIndex(b, []int{100}, func(sl *skiplist.SkipList[int64, int64], i int) {
		sl.Get(int64(i % 100))
	})
	benchmarkIndex(b, []int{1000000}, func(sl *skiplist.SkipList[int64, int64], i int) {
		sl.Get(int64(i % 1000000))
	})
}

func BenchmarkSkipList_Delete(b *testing.B) {
	benchmarkIndex(b, []int{100}, func(sl *skiplist.SkipList[int64, int64], i int) {
		_, _ = sl.Delete(int64(i % 100))
	})
	benchmarkIndex(b, []int{1000000}, func(sl *skiplist.SkipList[int64, int64], i int) {
		_, _ = sl.Delete(int64(i % 1000000))
	})
}

func BenchmarkSkipList_Min(b *testing.B) {
	benchmark(b, []int{100, 1000000}, func(sl *skiplist.SkipList[int64, int64]) {
		_, _, _ = sl.Min()
	})
}

func BenchmarkSkipList_Max(b *testing.B) {
	benchmark(b, []int{100, 1000000}, func(sl *skiplist.SkipList[int64, int64]) {
		_, _, _ = sl.Max()
	})
}

func BenchmarkSkipList_Floor(b *testing.B) {
	benchmark(b, []int{100}, func(sl *skiplist.SkipList[int64, int64]) {
		_, _, _ = sl.Floor(50)
	})
	benchmark(b, []int{1000000}, func(sl *skiplist.SkipList[int64, int64]) {
		_, _, _ = sl.Floor(500000)
	})
}

func BenchmarkSkipList_Ceiling(b *testing.B) {
	benchmark(b, []int{100}, func(sl *skiplist.SkipList[int64, int64]) {
		_, _, _ = sl.Ceiling(50)
	})
	benchmark(b, []int{1000000}, func(sl *skiplist.SkipList[int64, int64]) {
		_, _, _ = sl.Ceiling(500000)
	})
}

func BenchmarkSkipList_Keys(b *testing.B) {
	benchmark(b, []int{100, 1000000}, func(sl *skiplist.SkipList[int64, int64]) {
		_ = sl.Keys()
	})
}

func BenchmarkSkipList_Values(b *testing.B) {
	benchmark(b, []int{100, 1000000}, func(sl *skiplist.SkipList[int64, int64]) {
		_ = sl.Values()
	})
}

func BenchmarkSkipList_Range(b *testing.B) {
	benchmark(b, []int{100, 1000000}, func(sl *skiplist.SkipList[int64, int64]) {
		sl.Range(func(i1, i2 int64) bool {
			_, _ = i1, i2
			return true
		})
	})
}

func BenchmarkSkipList_Clone(b *testing.B) {
	benchmark(b, []int{100, 1000000}, func(sl *skiplist.SkipList[int64, int64]) {
		_ = sl.Clone()
	})
}

func BenchmarkSkipList_MarshalJSON(b *testing.B) {
	benchmark(b, []int{100, 1000000}, func(sl *skiplist.SkipList[int64, int64]) {
		_, _ = sl.MarshalJSON()
	})
}
