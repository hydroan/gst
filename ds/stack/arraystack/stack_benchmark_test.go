package arraystack_test

import (
	"fmt"
	"testing"

	"github.com/hydroan/gst/ds/stack/arraystack"
)

func createStack(b *testing.B, size int, safe bool) *arraystack.Stack[int] {
	b.Helper()
	cmp := func(a, b int) int { return a - b }
	var s *arraystack.Stack[int]
	var err error
	if safe {
		s, err = arraystack.New(cmp, arraystack.WithSafe[int]())
	} else {
		s, err = arraystack.New(cmp)
	}
	if err != nil {
		b.Fatal(err)
	}
	for i := range size {
		s.Push(i)
	}
	return s
}

func benchmark(b *testing.B, hasConcUnsafe bool, sizes []int, do func(s *arraystack.Stack[int])) {
	b.Helper()
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			b.Run("single unsafe", func(b *testing.B) {
				s := createStack(b, size, false)
				b.ResetTimer()
				for range b.N {
					do(s)
				}
			})
			b.Run("single safe", func(b *testing.B) {
				s := createStack(b, size, true)
				b.ResetTimer()
				for range b.N {
					do(s)
				}
			})
			if hasConcUnsafe {
				b.Run("concur unsafe", func(b *testing.B) {
					s := createStack(b, size, false)
					b.ResetTimer()
					b.RunParallel(func(p *testing.PB) {
						for p.Next() {
							do(s)
						}
					})
				})
			}
			b.Run("concur safe", func(b *testing.B) {
				s := createStack(b, size, true)
				b.ResetTimer()
				b.RunParallel(func(p *testing.PB) {
					for p.Next() {
						do(s)
					}
				})
			})
		})
	}
}

func BenchmarkArrayStack_Push(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *arraystack.Stack[int]) {
		s.Push(0)
	})
}

func BenchmarkArrayStack_Pop(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *arraystack.Stack[int]) {
		_, _ = s.Pop()
	})
}

func BenchmarkArrayStack_Peek(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *arraystack.Stack[int]) {
		_, _ = s.Peek()
	})
}

func BenchmarkArrayStack_Len(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *arraystack.Stack[int]) {
		_ = s.Len()
	})
}

func BenchmarkArrayStack_IsEmpty(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *arraystack.Stack[int]) {
		_ = s.IsEmpty()
	})
}

func BenchmarkArrayStack_Values(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *arraystack.Stack[int]) {
		_ = s.Values()
	})
}

func BenchmarkArrayStack_Clear(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *arraystack.Stack[int]) {
		s.Clear()
	})
}

func BenchmarkArrayStack_Clone(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *arraystack.Stack[int]) {
		s.Clone()
	})
}

func BenchmarkArrayStack_String(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *arraystack.Stack[int]) {
		_ = s.String()
	})
}

func BenchmarkArrayStack_MarshalJSON(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *arraystack.Stack[int]) {
		_, _ = s.MarshalJSON()
	})
}

func BenchmarkArrayStack_UnmarshalJSON(b *testing.B) {
	data := []byte("[1,2,3]")
	benchmark(b, false, []int{100, 100000}, func(s *arraystack.Stack[int]) {
		_ = s.UnmarshalJSON(data)
	})
}
