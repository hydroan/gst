package splaytree_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/hydroan/gst/ds/tree/splaytree"
)

func createTree(b *testing.B, size int, safe bool) *splaytree.Tree[float64, float64] {
	b.Helper()
	var t *splaytree.Tree[float64, float64]
	var err error
	if safe {
		t, err = splaytree.NewOrderedKeys(splaytree.WithSafe[float64, float64]())
	} else {
		t, err = splaytree.NewOrderedKeys[float64, float64]()
	}
	for i := range size {
		t.Put(float64(i), float64(i))
	}
	if err != nil {
		b.Fatalf("failed to create splay tree: %v", err)
	}
	return t
}

func benchmark(b *testing.B, hasConcUnsafe bool, sizes []int, do func(t *splaytree.Tree[float64, float64])) {
	b.Helper()
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			b.Run("single unsafe", func(b *testing.B) {
				t := createTree(b, size, false)
				b.ResetTimer()
				for range b.N {
					do(t)
				}
			})
			b.Run("single safe", func(b *testing.B) {
				t := createTree(b, size, true)
				b.ResetTimer()
				for range b.N {
					do(t)
				}
			})

			if hasConcUnsafe {
				b.Run("concur unsafe", func(b *testing.B) {
					t := createTree(b, size, false)
					b.ResetTimer()
					b.RunParallel(func(p *testing.PB) {
						for p.Next() {
							do(t)
						}
					})
				})
			}
			b.Run("concur safe", func(b *testing.B) {
				t := createTree(b, size, true)
				b.ResetTimer()
				b.RunParallel(func(p *testing.PB) {
					for p.Next() {
						do(t)
					}
				})
			})
		})
	}
}

func BenchmarkSplayTree_Put(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		t.Put(float64(time.Now().UnixNano()), float64(time.Now().UnixNano()))
	})
}

func BenchmarkSplayTree_Get(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		_, _ = t.Get(0)
	})
}

func BenchmarkSplayTree_Delete(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		t.Delete(0)
	})
}

func BenchmarkSplayTree_Size(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		_ = t.Size()
	})
}

func BenchmarkSplayTree_IsEmpty(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		_ = t.IsEmpty()
	})
}

func BenchmarkSplayTree_Clear(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		t.Clear()
	})
}

func BenchmarkSplayTree_Keys(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		_ = t.Keys()
	})
}

func BenchmarkSplayTree_Values(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		_ = t.Values()
	})
}

func BenchmarkSplayTree_Min(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		_, _, _ = t.Min()
	})
}

func BenchmarkSplayTree_Max(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		_, _, _ = t.Max()
	})
}

func BenchmarkSplayTree_Floor(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		_, _, _ = t.Floor(0)
	})
}

func BenchmarkSplayTree_Ceiling(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		_, _, _ = t.Ceiling(0)
	})
}

func BenchmarkPreOrder(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		t.PreOrder(func(key, value float64) bool {
			_, _ = key, value
			return true
		})
	})
}

func BenchmarkInOrder(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		t.InOrder(func(key, value float64) bool {
			_, _ = key, value
			return true
		})
	})
}

func BenchmarkPostOrder(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		t.PostOrder(func(key, value float64) bool {
			_, _ = key, value
			return true
		})
	})
}

func BenchmarkLevelOrder(b *testing.B) {
	benchmark(b, false, []int{10, 100000}, func(t *splaytree.Tree[float64, float64]) {
		t.LevelOrder(func(key, value float64) bool {
			_, _ = key, value
			return true
		})
	})
}

func BenchmarkSplayTree_String(b *testing.B) {
	benchmark(b, false, []int{1000}, func(t *splaytree.Tree[float64, float64]) {
		_ = t.String()
	})
}
