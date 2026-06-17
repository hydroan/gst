package binaryheap_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/hydroan/gst/ds/heap/binaryheap"
)

func createSafeHeap(b *testing.B, size int, safe bool) *binaryheap.Heap[int] {
	b.Helper()
	var h *binaryheap.Heap[int]
	var err error
	if safe {
		h, err = binaryheap.NewOrdered(binaryheap.WithSafe[int]())
	} else {
		h, err = binaryheap.NewOrdered[int]()
	}
	if err != nil {
		b.Fatalf("failed to create binary heap")
	}
	for i := range size {
		h.Push(i)
	}
	return h
}

func benchmark(b *testing.B, sizes []int, do func(h *binaryheap.Heap[int])) {
	b.Helper()
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			b.Run("single unsafe", func(b *testing.B) {
				h := createSafeHeap(b, size, false)
				b.ResetTimer()
				for range b.N {
					do(h)
				}
			})
			b.Run("single safe", func(b *testing.B) {
				h := createSafeHeap(b, size, true)
				b.ResetTimer()
				for range b.N {
					do(h)
				}
			})
			b.Run("concur safe", func(b *testing.B) {
				h := createSafeHeap(b, size, true)
				b.ResetTimer()
				b.RunParallel(func(p *testing.PB) {
					for p.Next() {
						do(h)
					}
				})
			})
		})
	}
}

func BenchmarkBinaryHeap_Push(b *testing.B) {
	benchmark(b, []int{10, 100000}, func(h *binaryheap.Heap[int]) {
		h.Push(int(time.Now().UnixNano()))
	})
}

func BenchmarkBinaryHeap_Pop(b *testing.B) {
	benchmark(b, []int{10, 100000}, func(h *binaryheap.Heap[int]) {
		_, _ = h.Pop()
	})
}

func BenchmarkBinaryHeap_Peek(b *testing.B) {
	benchmark(b, []int{10, 100000}, func(h *binaryheap.Heap[int]) {
		_, _ = h.Peek()
	})
}

func BenchmarkBinaryHeap_Values(b *testing.B) {
	benchmark(b, []int{10, 100000}, func(h *binaryheap.Heap[int]) {
		_ = h.Values()
	})
}

func BenchmarkBinaryHeap_Clone(b *testing.B) {
	benchmark(b, []int{10, 100000}, func(h *binaryheap.Heap[int]) {
		_ = h.Clone()
	})
}

func BenchmarkBinaryHeap_Range(b *testing.B) {
	benchmark(b, []int{10, 100000}, func(h *binaryheap.Heap[int]) {
		h.Range(func(e int) bool {
			_ = e
			return true
		})
	})
}
