package circularbuffer_test

import (
	"encoding/json"
	"fmt"
	"testing"

	cb "github.com/hydroan/gst/ds/queue/circularbuffer"
)

func createCircularBuffer(b *testing.B, size int, safe bool, drop bool) *cb.CircularBuffer[int] {
	b.Helper()
	ops := make([]cb.Option[int], 0)
	if safe {
		ops = append(ops, cb.WithSafe[int]())
	}
	if drop {
		ops = append(ops, cb.WithDrop[int]())
	}

	cb, err := cb.New(10, ops...)
	if err != nil {
		b.Fatalf("failed to create circular buffer: %v", err)
	}

	for i := range size {
		cb.Enqueue(i)
	}

	return cb
}

func createCircularBuffer2(b *testing.B, size int, safe bool) *cb.CircularBuffer[int] {
	b.Helper()
	ops := make([]cb.Option[int], 0)
	if safe {
		ops = append(ops, cb.WithSafe[int]())
	}

	cb, err := cb.New(size, ops...)
	if err != nil {
		b.Fatalf("failed to create circular buffer: %v", err)
	}

	for i := range size {
		cb.Enqueue(i)
	}

	return cb
}

func benchmark(b *testing.B, sizes []int, do func(cb *cb.CircularBuffer[int])) {
	b.Helper()
	b.Run("overwrite", func(b *testing.B) {
		for _, size := range sizes {
			b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
				b.Run("single unsafe", func(b *testing.B) {
					cb := createCircularBuffer(b, size, false, false)
					b.ResetTimer()
					for range b.N {
						do(cb)
					}
				})
				b.Run("single safe", func(b *testing.B) {
					cb := createCircularBuffer(b, size, true, false)
					b.ResetTimer()
					for range b.N {
						do(cb)
					}
				})
				b.Run("concur safe", func(b *testing.B) {
					cb := createCircularBuffer(b, size, true, false)
					b.ResetTimer()
					b.RunParallel(func(p *testing.PB) {
						for p.Next() {
							do(cb)
						}
					})
				})
			})
		}
	})
	b.Run("drop", func(b *testing.B) {
		for _, size := range sizes {
			b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
				b.Run("single unsafe", func(b *testing.B) {
					cb := createCircularBuffer(b, size, false, true)
					b.ResetTimer()
					for range b.N {
						do(cb)
					}
				})
				b.Run("single safe", func(b *testing.B) {
					cb := createCircularBuffer(b, size, true, true)
					b.ResetTimer()
					for range b.N {
						do(cb)
					}
				})
				b.Run("concur safe", func(b *testing.B) {
					cb := createCircularBuffer(b, size, true, true)
					b.ResetTimer()
					b.RunParallel(func(p *testing.PB) {
						for p.Next() {
							do(cb)
						}
					})
				})
			})
		}
	})
}

func benchmark2(b *testing.B, sizes []int, do func(cb *cb.CircularBuffer[int])) {
	b.Helper()
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			b.Run("single unsafe", func(b *testing.B) {
				cb := createCircularBuffer2(b, size, false)
				b.ResetTimer()
				for range b.N {
					do(cb)
				}
			})
			b.Run("single safe", func(b *testing.B) {
				cb := createCircularBuffer2(b, size, true)
				b.ResetTimer()
				for range b.N {
					do(cb)
				}
			})
			b.Run("concur safe", func(b *testing.B) {
				cb := createCircularBuffer2(b, size, true)
				b.ResetTimer()
				b.RunParallel(func(p *testing.PB) {
					for p.Next() {
						do(cb)
					}
				})
			})
		})
	}
}

func BenchmarkCircularBuffer_Enqueue(b *testing.B) {
	benchmark(b, []int{100, 1000000}, func(cb *cb.CircularBuffer[int]) {
		cb.Enqueue(0)
	})
}

func BenchmarkCircularBuffer_Dequeue(b *testing.B) {
	benchmark2(b, []int{100, 100000}, func(cb *cb.CircularBuffer[int]) {
		_, _ = cb.Dequeue()
	})
}

func BenchmarkCircularBuffer_Peek(b *testing.B) {
	benchmark2(b, []int{100, 100000}, func(cb *cb.CircularBuffer[int]) {
		_, _ = cb.Peek()
	})
}

func BenchmarkCircularBuffer_Slice(b *testing.B) {
	benchmark2(b, []int{100, 100000}, func(cb *cb.CircularBuffer[int]) {
		_ = cb.Slice()
	})
}

func BenchmarkCircularBuffer_Clone(b *testing.B) {
	benchmark2(b, []int{100, 100000}, func(cb *cb.CircularBuffer[int]) {
		_ = cb.Clone()
	})
}

func BenchmarkCircularBuffer_Range(b *testing.B) {
	benchmark2(b, []int{100, 100000}, func(cb *cb.CircularBuffer[int]) {
		cb.Range(func(e int) bool {
			_ = e
			return true
		})
	})
}

func BenchmarkCircularBuffer_MarshalJSON(b *testing.B) {
	benchmark2(b, []int{100, 100000}, func(cb *cb.CircularBuffer[int]) {
		_, _ = json.Marshal(cb)
	})
}

func BenchmarkCircularBuffer_UnmarshalJSON(b *testing.B) {
	dst, err := cb.New[int](10)
	if err != nil {
		b.Fatal(err)
	}
	bytes := []byte("[1, 2, 3, 4, 5, 6, 7, 8, 9, 10]")
	benchmark2(b, []int{100, 100000}, func(cb *cb.CircularBuffer[int]) {
		_ = json.Unmarshal(bytes, dst)
	})
}

func BenchmarkCircularBuffer_String(b *testing.B) {
	benchmark2(b, []int{100, 100000}, func(cb *cb.CircularBuffer[int]) {
		_ = cb.String()
	})
}
