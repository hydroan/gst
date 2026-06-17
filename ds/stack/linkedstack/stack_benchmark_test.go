package linkedstack_test

import (
	"fmt"
	"testing"

	"github.com/hydroan/gst/ds/stack/linkedstack"
)

func createStack(b *testing.B, size int, safe bool) *linkedstack.Stack[int] {
	b.Helper()
	var s *linkedstack.Stack[int]
	var err error
	if safe {
		s, err = linkedstack.New(linkedstack.WithSafe[int]())
	} else {
		s, err = linkedstack.New[int]()
	}
	if err != nil {
		b.Fatal(err)
	}
	for i := range size {
		s.Push(i)
	}
	return s
}

func benchmark(b *testing.B, hasConcUnsafe bool, sizes []int, do func(s *linkedstack.Stack[int])) {
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

func BenchmarkLinkedlistStack_Push(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *linkedstack.Stack[int]) {
		s.Push(0)
	})
}

func BenchmarkLinkedlistStack_Pop(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *linkedstack.Stack[int]) {
		_, _ = s.Pop()
	})
}

func BenchmarkLinkedlistStack_Peek(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *linkedstack.Stack[int]) {
		_, _ = s.Peek()
	})
}

func BenchmarkLinkedlistStack_Len(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *linkedstack.Stack[int]) {
		_ = s.Len()
	})
}

func BenchmarkLinkedlistStack_IsEmpty(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *linkedstack.Stack[int]) {
		_ = s.IsEmpty()
	})
}

func BenchmarkLinkedlistStack_Values(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *linkedstack.Stack[int]) {
		_ = s.Values()
	})
}

func BenchmarkLinkedlistStack_Clear(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *linkedstack.Stack[int]) {
		s.Clear()
	})
}

func BenchmarkLinkedlistStack_Clone(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *linkedstack.Stack[int]) {
		s.Clone()
	})
}

func BenchmarkLinkedlistStack_String(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *linkedstack.Stack[int]) {
		_ = s.String()
	})
}

func BenchmarkLinkedlistStack_MarshalJSON(b *testing.B) {
	benchmark(b, false, []int{100, 100000}, func(s *linkedstack.Stack[int]) {
		_, _ = s.MarshalJSON()
	})
}

func BenchmarkLinkedlistStack_UnmarshalJSON(b *testing.B) {
	data := []byte("[1,2,3]")
	benchmark(b, false, []int{100, 100000}, func(s *linkedstack.Stack[int]) {
		_ = s.UnmarshalJSON(data)
	})
}
