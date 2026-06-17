package linkedqueue_test

import (
	"fmt"
	"testing"

	"github.com/hydroan/gst/ds/queue/linkedqueue"
)

func createQueue(b *testing.B, size int, safe bool) *linkedqueue.Queue[int] {
	b.Helper()
	var q *linkedqueue.Queue[int]
	var err error
	if safe {
		q, err = linkedqueue.New(intCmp, linkedqueue.WithSafe[int]())
	} else {
		q, err = linkedqueue.New(intCmp)
	}
	for i := range size {
		q.Enqueue(i)
	}
	if err != nil {
		b.Fatalf("failed to create queue: %v", err)
	}
	return q
}

func benchmark(b *testing.B, hasConcUnsafe bool, sizes []int, do func(q *linkedqueue.Queue[int])) {
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

			if hasConcUnsafe {
				b.Run("concur unsafe", func(b *testing.B) {
					q := createQueue(b, size, false)
					b.ResetTimer()
					b.RunParallel(func(p *testing.PB) {
						for p.Next() {
							do(q)
						}
					})
				})
			}
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

func BenchmarkArrayQueue_Enqueue(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(q *linkedqueue.Queue[int]) {
		q.Enqueue(0)
	})
}

func BenchmarkArrayQueue_Dequeue(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(q *linkedqueue.Queue[int]) {
		_, _ = q.Dequeue()
	})
}

func BenchmarkArrayQueue_Peek(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(q *linkedqueue.Queue[int]) {
		_, _ = q.Peek()
	})
}

func BenchmarkArrayQueue_IsEmpty(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(q *linkedqueue.Queue[int]) {
		_ = q.IsEmpty()
	})
}

func BenchmarkArrayQueue_Len(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(q *linkedqueue.Queue[int]) {
		_ = q.Len()
	})
}

func BenchmarkArrayQueue_Values(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(q *linkedqueue.Queue[int]) {
		_ = q.Values()
	})
}

func BenchmarkArrayQueue_Clear(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(q *linkedqueue.Queue[int]) {
		q.Clear()
	})
}

func BenchmarkArrayQueue_Clone(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(q *linkedqueue.Queue[int]) {
		_ = q.Clone()
	})
}

func BenchmarkArrayQueue_String(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(q *linkedqueue.Queue[int]) {
		_ = q.String()
	})
}

func BenchmarkArrayQueue_MarshalJSON(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(q *linkedqueue.Queue[int]) {
		_, _ = q.MarshalJSON()
	})
}

func BenchmarkArrayQueue_UnmarshalJSON(b *testing.B) {
	data := []byte("[1,2,3]")
	benchmark(b, false, []int{100, 100000}, func(q *linkedqueue.Queue[int]) {
		_ = q.UnmarshalJSON(data)
	})
}
