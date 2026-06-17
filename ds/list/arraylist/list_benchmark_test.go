package arraylist_test

import (
	"fmt"
	"testing"

	"github.com/hydroan/gst/ds/list/arraylist"
)

func createList(b *testing.B, size int, safe bool) *arraylist.List[int] {
	b.Helper()
	var list *arraylist.List[int]
	var err error

	if safe {
		list, err = arraylist.New(cmp, arraylist.WithSafe[int]())
	} else {
		list, err = arraylist.New(cmp)
	}
	if err != nil {
		b.Fatalf("failed to create list: %v", err)
	}
	for i := range size {
		list.Append(i)
	}

	return list
}

func benchmark(b *testing.B, hasConcUnsafe bool, sizes []int, fn func(list *arraylist.List[int])) {
	b.Helper()
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			b.Run("single unsafe", func(b *testing.B) {
				list := createList(b, size, false)
				b.ResetTimer()
				for range b.N {
					fn(list)
				}
			})

			b.Run("single safe", func(b *testing.B) {
				list := createList(b, size, true)
				b.ResetTimer()
				for range b.N {
					fn(list)
				}
			})

			if hasConcUnsafe {
				b.Run("concur unsafe", func(b *testing.B) {
					list := createList(b, size, false)
					b.ResetTimer()
					b.RunParallel(func(pb *testing.PB) {
						for pb.Next() {
							fn(list)
						}
					})
				})
			}

			b.Run("concur safe", func(b *testing.B) {
				list := createList(b, size, true)
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						fn(list)
					}
				})
			})
		})
	}
}

func BenchmarkArrayList_Get(b *testing.B) {
	benchmark(b, false, []int{10, 1000000}, func(list *arraylist.List[int]) {
		_, _ = list.Get(-1)
	})
}

func BenchmarkArrayList_AppendOne(b *testing.B) {
	benchmark(b, false, []int{10, 1000000}, func(list *arraylist.List[int]) {
		list.Append(0)
	})
}

func BenchmarkArrayList_AppendTen(b *testing.B) {
	benchmark(b, false, []int{10, 1000000}, func(list *arraylist.List[int]) {
		list.Append(0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
	})
}

func BenchmarkArrayList_Insert(b *testing.B) {
	benchmark(b, false, []int{10000}, func(list *arraylist.List[int]) {
		list.Insert(10, 0)
	})
}

func BenchmarkArrayList_Set(b *testing.B) {
	benchmark(b, false, []int{1000000}, func(list *arraylist.List[int]) {
		list.Set(10, 0)
	})
}

func BenchmarkArrayList_Remove(b *testing.B) {
	benchmark(b, false, []int{100, 1000}, func(list *arraylist.List[int]) {
		list.Remove(-1)
	})
}

func BenchmarkArrayList_RemoveAt(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(list *arraylist.List[int]) {
		list.RemoveAt(10)
	})
}

func BenchmarkArrayList_Clear(b *testing.B) {
	benchmark(b, false, []int{1000}, func(list *arraylist.List[int]) {
		list.Clear()
	})
}

func BenchmarkArrayList_Contains_Worst(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(list *arraylist.List[int]) {
		list.Contains(-1)
	})
}

func BenchmarkArrayList_Contains_Best(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(list *arraylist.List[int]) {
		list.Contains(0)
	})
}

func BenchmarkArrayList_Contains_Range(b *testing.B) {
	benchmark(b, false, []int{100}, func(list *arraylist.List[int]) {
		for i := range 100 {
			list.Contains(i)
		}
	})
	benchmark(b, false, []int{1000}, func(list *arraylist.List[int]) {
		for i := range 1000 {
			list.Contains(i)
		}
	})
}

func BenchmarkArrayList_Values(b *testing.B) {
	benchmark(b, false, []int{100, 10000}, func(list *arraylist.List[int]) {
		_ = list.Values()
	})
}

func BenchmarkArrayList_IndexOf_Worst(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(list *arraylist.List[int]) {
		list.IndexOf(-1)
	})
}

func BenchmarkArrayList_IndexOf_Best(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(list *arraylist.List[int]) {
		list.IndexOf(0)
	})
}

func BenchmarkArrayList_IndexOf_Range(b *testing.B) {
	benchmark(b, false, []int{100}, func(list *arraylist.List[int]) {
		for i := range 100 {
			list.IndexOf(i)
		}
	})
	benchmark(b, false, []int{10000}, func(list *arraylist.List[int]) {
		for i := range 10000 {
			list.IndexOf(i)
		}
	})
}

func BenchmarkArrayList_IsEmpty(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(list *arraylist.List[int]) {
		list.IsEmpty()
	})
}

func BenchmarkArrayList_Len(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(list *arraylist.List[int]) {
		list.Len()
	})
}

func BenchmarkArrayList_Sort(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(list *arraylist.List[int]) {
		list.Sort()
	})
}

func BenchmarkArrayList_Swap(b *testing.B) {
	benchmark(b, false, []int{100}, func(list *arraylist.List[int]) {
		list.Swap(0, 99)
	})
	benchmark(b, false, []int{100000}, func(list *arraylist.List[int]) {
		list.Swap(0, 99999)
	})
}

func BenchmarkArrayList_Range(b *testing.B) {
	benchmark(b, false, []int{100, 10000}, func(list *arraylist.List[int]) {
		list.Range(func(v int) bool {
			_ = v
			return true
		})
	})
}
