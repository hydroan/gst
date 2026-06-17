package linkedlist_test

import (
	golist "container/list"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/ds/list/linkedlist"
)

func createLinkedList(b *testing.B, size int, safe bool) *linkedlist.List[int] {
	b.Helper()
	slice := make([]int, 0, size)
	for i := range size {
		slice = append(slice, i)
	}

	var list *linkedlist.List[int]
	var err error
	if safe {
		list, err = linkedlist.NewFromSlice(slice, linkedlist.WithSafe[int]())
	} else {
		list, err = linkedlist.NewFromSlice(slice)
	}
	if err != nil {
		b.Fatalf("failed to create list: %v", err)
	}
	return list
}

func createStdList(_ *testing.B, size int) *golist.List {
	list := golist.New()
	for i := range size {
		list.PushBack(i)
	}
	return list
}

func benchmark(b *testing.B, sizes []int, f1 func(list *linkedlist.List[int]), f2 func(list *golist.List)) {
	b.Helper()
	var mu sync.Mutex
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			if f1 != nil {
				b.Run("unsafe custom", func(b *testing.B) {
					l := createLinkedList(b, size, false)
					b.ResetTimer()
					for range b.N {
						f1(l)
					}
				})
			}
			if f2 != nil {
				b.Run("unsafe std", func(b *testing.B) {
					l := createStdList(b, size)
					b.ResetTimer()
					for range b.N {
						f2(l)
					}
				})
			}

			if f1 != nil {
				b.Run("safe custom", func(b *testing.B) {
					l := createLinkedList(b, size, true)
					b.ResetTimer()
					for range b.N {
						f1(l)
					}
				})
			}
			if f2 != nil {
				b.Run("safe std", func(b *testing.B) {
					l := createStdList(b, size)
					b.ResetTimer()
					for range b.N {
						mu.Lock()
						f2(l)
						mu.Unlock()
					}
				})
			}

			if f1 != nil {
				b.Run("safe conc custom", func(b *testing.B) {
					l := createLinkedList(b, size, true)
					b.ResetTimer()
					b.RunParallel(func(p *testing.PB) {
						for p.Next() {
							f1(l)
						}
					})
				})
			}

			if f2 != nil {
				b.Run("safe conc std", func(b *testing.B) {
					l := createStdList(b, size)
					b.ResetTimer()
					b.RunParallel(func(p *testing.PB) {
						for p.Next() {
							mu.Lock()
							f2(l)
							mu.Unlock()
						}
					})
				})
			}
		})
	}
}

func BenchmarkLinkedList_PushBack(b *testing.B) {
	benchmark(
		b, []int{10},
		func(list *linkedlist.List[int]) { _ = list.PushBack(0) },
		func(list *golist.List) { _ = list.PushBack(0) },
	)
}

func BenchmarkLinkedList_PushFront(b *testing.B) {
	benchmark(
		b, []int{10},
		func(list *linkedlist.List[int]) { _ = list.PushFront(0) },
		func(list *golist.List) { _ = list.PushBack(0) },
	)
}

func BenchmarkLinkedList_InsertAfter(b *testing.B) {
	for _, size := range []int{10, 10000} {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			benchmarkInsertAfter(b, size)
		})
	}
}

func benchmarkInsertAfter(b *testing.B, _ int) {
	b.Helper()
	var mu sync.Mutex

	b.Run("unsafe custom", func(b *testing.B) {
		l := createLinkedList(b, 0, false)
		l.PushBack(0)
		l.PushBack(1)
		l.PushBack(2)
		n := l.Head.Next
		b.ResetTimer()
		for range b.N {
			_ = l.InsertAfter(n, 0)
		}
	})
	b.Run("unsafe std", func(b *testing.B) {
		l := golist.New()
		l.PushBack(0)
		l.PushBack(1)
		l.PushBack(2)
		e := l.Front().Next()
		b.ResetTimer()
		for range b.N {
			l.InsertAfter(e, &golist.Element{Value: 0})
		}
	})

	b.Run("safe custom", func(b *testing.B) {
		l := createLinkedList(b, 0, true)
		l.PushBack(0)
		l.PushBack(1)
		l.PushBack(2)
		n := l.Head.Next
		b.ResetTimer()
		for range b.N {
			_ = l.InsertAfter(n, 0)
		}
	})
	b.Run("safe std", func(b *testing.B) {
		l := golist.New()
		l.PushBack(0)
		l.PushBack(1)
		l.PushBack(2)
		e := l.Front().Next()
		b.ResetTimer()
		for range b.N {
			mu.Lock()
			l.InsertAfter(e, &golist.Element{Value: 0})
			mu.Unlock()
		}
	})

	b.Run("safe conc custom", func(b *testing.B) {
		l := createLinkedList(b, 0, true)
		l.PushBack(0)
		l.PushBack(1)
		l.PushBack(2)
		n := l.Head.Next
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				_ = l.InsertAfter(n, 0)
			}
		})
	})
	b.Run("safe conc std", func(b *testing.B) {
		l := golist.New()
		l.PushBack(0)
		l.PushBack(1)
		l.PushBack(2)
		e := l.Front().Next()
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				mu.Lock()
				l.InsertAfter(e, &golist.Element{Value: 0})
				mu.Unlock()
			}
		})
	})
}

func BenchmarkLinkedList_InsertBefore(b *testing.B) {
	for _, size := range []int{10, 10000} {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			benchmarkInsertBefore(b, size)
		})
	}
}

func benchmarkInsertBefore(b *testing.B, _ int) {
	b.Helper()
	var mu sync.Mutex

	b.Run("unsafe custom", func(b *testing.B) {
		l := createLinkedList(b, 0, false)
		l.PushBack(0)
		l.PushBack(1)
		l.PushBack(2)
		n := l.Head.Next
		b.ResetTimer()
		for range b.N {
			_ = l.InsertBefore(n, 0)
		}
	})
	b.Run("unsafe std", func(b *testing.B) {
		l := golist.New()
		l.PushBack(0)
		l.PushBack(1)
		l.PushBack(2)
		e := l.Front().Next()
		b.ResetTimer()
		for range b.N {
			l.InsertAfter(e, &golist.Element{Value: 0})
		}
	})

	b.Run("safe custom", func(b *testing.B) {
		l := createLinkedList(b, 0, true)
		l.PushBack(0)
		l.PushBack(1)
		l.PushBack(2)
		n := l.Head.Next
		b.ResetTimer()
		for range b.N {
			_ = l.InsertBefore(n, 0)
		}
	})
	b.Run("safe std", func(b *testing.B) {
		l := golist.New()
		l.PushBack(0)
		l.PushBack(1)
		l.PushBack(2)
		e := l.Front().Next()
		b.ResetTimer()
		for range b.N {
			mu.Lock()
			l.InsertAfter(e, &golist.Element{Value: 0})
			mu.Unlock()
		}
	})

	b.Run("safe conc custom", func(b *testing.B) {
		l := createLinkedList(b, 0, true)
		l.PushBack(0)
		l.PushBack(1)
		l.PushBack(2)
		n := l.Head.Next
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				_ = l.InsertBefore(n, 0)
			}
		})
	})
	b.Run("safe conc std", func(b *testing.B) {
		l := golist.New()
		l.PushBack(0)
		l.PushBack(1)
		l.PushBack(2)
		e := l.Front().Next()
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				mu.Lock()
				l.InsertAfter(e, &golist.Element{Value: 0})
				mu.Unlock()
			}
		})
	})
}

func BenchmarkLinkedList_PopBack(b *testing.B) {
	benchmark(
		b, []int{10000000},
		func(list *linkedlist.List[int]) {
			_ = list.PopBack()
		},
		func(list *golist.List) {
			if list.Back() != nil {
				_ = list.Remove(list.Back())
			} else {
				_ = list.Remove(&golist.Element{Value: -1})
			}
		},
	)
}

func BenchmarkLinkedList_PopFront(b *testing.B) {
	benchmark(
		b, []int{10000000},
		func(list *linkedlist.List[int]) {
			_ = list.PopFront()
		},
		func(list *golist.List) {
			if list.Front() != nil {
				_ = list.Remove(list.Front())
			} else {
				_ = list.Remove(&golist.Element{Value: -1})
			}
		},
	)
}

func BenchmarkLinkedList_FindWorstCase(b *testing.B) {
	equal := func(a, b int) bool { return a == b }
	benchmark(
		b, []int{10, 10000},
		func(list *linkedlist.List[int]) { list.Find(-1, equal) },
		func(list *golist.List) { stdListFind(list, -1, equal) },
	)
}

func BenchmarkLinkedList_FindBestCase(b *testing.B) {
	equal := func(a, b int) bool { return a == b }
	benchmark(
		b, []int{10, 10000},
		func(list *linkedlist.List[int]) { list.Find(0, equal) },
		func(list *golist.List) { stdListFind(list, 0, equal) },
	)
}

func stdListFind(list *golist.List, v any, equal func(int, int) bool) (_v any) {
	for e := list.Front(); e != nil; e = e.Next() {
		if equal(v.(int), e.Value.(int)) { //nolint:errcheck
			return v
		}
	}
	return _v
}

func BenchmarkLinkedList_Reverse(b *testing.B) {
	benchmark(b, []int{10, 10000}, func(list *linkedlist.List[int]) {
		list.Reverse()
	}, nil)
}

func BenchmarkLinkedList_Merge(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			benchmarkMerge(b, size)
		})
	}
}

func benchmarkMerge(b *testing.B, size int) {
	b.Helper()
	b.Run("unsafe", func(b *testing.B) {
		l1 := createLinkedList(b, size, false)
		b.ResetTimer()
		for range b.N {
			l2 := createLinkedList(b, size, false)
			l1.Merge(l2)
		}
	})

	b.Run("safe", func(b *testing.B) {
		l1 := createLinkedList(b, size, true)
		b.ResetTimer()
		for range b.N {
			l2 := createLinkedList(b, size, true)
			l1.Merge(l2)
		}
	})
}

func BenchmarkLinkedList_MergeSorted(b *testing.B) {
	// go test -bench 'MergeSorted' ./ds/list/linkedlist/ -benchtime=1000x
	for _, size := range []int{10} {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			benchmarkMergeSorted(b, size)
		})
	}
}

func benchmarkMergeSorted(b *testing.B, size int) {
	b.Helper()
	cmp := func(a, b int) int {
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}

	b.Run("unsafe", func(b *testing.B) {
		slice := make([]int, 0, size)
		r := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano())))
		for range size {
			slice = append(slice, r.IntN(size))
		}
		l1, err := linkedlist.NewFromSlice(slice)
		if err != nil {
			b.Fatalf("failed to create list: %v", err)
		}
		b.ResetTimer()
		for range b.N {
			b.StopTimer()
			l2, err := linkedlist.NewFromSlice(slice)
			if err != nil {
				b.Fatalf("failed to create list: %v", err)
			}
			b.StartTimer()
			l1.MergeSorted(l2, cmp)
		}
	})

	b.Run("safe", func(b *testing.B) {
		slice := make([]int, 0, size)
		r := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano())))
		for range size {
			slice = append(slice, r.IntN(size))
		}
		l1, err := linkedlist.NewFromSlice(slice, linkedlist.WithSafe[int]())
		if err != nil {
			b.Fatalf("failed to create list: %v", err)
		}
		b.ResetTimer()
		for range b.N {
			b.StopTimer()
			l2, err := linkedlist.NewFromSlice(slice, linkedlist.WithSafe[int]())
			if err != nil {
				b.Fatalf("failed to create list: %v", err)
			}
			b.StartTimer()
			l1.MergeSorted(l2, cmp)
		}
	})
}
