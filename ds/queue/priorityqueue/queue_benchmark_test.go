package priorityqueue_test

import (
	"fmt"
	"testing"

	pq "github.com/hydroan/gst/ds/queue/priorityqueue"
)

func createQueue(b *testing.B, size int, safe bool) *pq.Queue[int] {
	b.Helper()
	var q *pq.Queue[int]
	var err error
	if safe {
		q, err = pq.New(intCmp, pq.WithSafe[int]())
	} else {
		q, err = pq.New(intCmp)
	}
	if err != nil {
		b.Fatalf("failed to create queue: %v", err)
	}
	for i := range size {
		q.Enqueue(i)
	}
	return q
}

func benchmark(b *testing.B, sizes []int, do func(q *pq.Queue[int])) {
	b.Helper()
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			b.Run("single unsafe", func(b *testing.B) {
				q := createQueue(b, size, false)
				b.ResetTimer()
				for range b.N {
					do(q)
				}
			})
			b.Run("single safe", func(b *testing.B) {
				q := createQueue(b, size, true)
				b.ResetTimer()
				for range b.N {
					do(q)
				}
			})
			b.Run("concur safe", func(b *testing.B) {
				q := createQueue(b, size, true)
				b.ResetTimer()
				b.RunParallel(func(p *testing.PB) {
					for p.Next() {
						do(q)
					}
				})
			})
		})
	}
}

func BenchmarkPriorityQueue_Enqueue(b *testing.B) {
	benchmark(b, []int{10, 100000}, func(q *pq.Queue[int]) {
		q.Enqueue(0)
	})
}

func BenchmarkPriorityQueue_Dequeue(b *testing.B) {
	benchmark(b, []int{10, 100000}, func(q *pq.Queue[int]) {
		_, _ = q.Dequeue()
	})
}

func BenchmarkPriorityQueue_Peek(b *testing.B) {
	benchmark(b, []int{10, 100000}, func(q *pq.Queue[int]) {
		_, _ = q.Peek()
	})
}

func BenchmarkPriorityQueue_Values(b *testing.B) {
	benchmark(b, []int{10, 100000}, func(q *pq.Queue[int]) {
		_ = q.Values()
	})
}

func BenchmarkPriorityQueue_Clone(b *testing.B) {
	benchmark(b, []int{10, 100000}, func(q *pq.Queue[int]) {
		_ = q.Clone()
	})
}
