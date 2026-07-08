package mapset_test

import (
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/ds/mapset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intCmp(a, b int) int {
	return a - b
}

func stringCmp(a, b string) int {
	if a > b {
		return 1
	} else if a < b {
		return -1
	}
	return 0
}

func TestNew(t *testing.T) {
	s, err := mapset.New[int]()
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.True(t, s.IsEmpty())
}

func TestNewFromSlice(t *testing.T) {
	s, err := mapset.NewFromSlice([]int{1, 2, 3, 4}, mapset.WithSorted(intCmp))
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, 4, s.Len())
	assert.Equal(t, []int{1, 2, 3, 4}, s.Slice())
}

func TestNewFromMapKeys(t *testing.T) {
	m := map[int]string{1: "a", 2: "b", 3: "c"}
	s, err := mapset.NewFromMapKeys(m, mapset.WithSorted(intCmp))
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, 3, s.Len())
	assert.Equal(t, []int{1, 2, 3}, s.Slice())
}

func TestNewFromMapValues(t *testing.T) {
	m := map[int]string{1: "a", 2: "b", 3: "c"}
	s, err := mapset.NewFromMapValues(m, mapset.WithSorted(stringCmp))
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, 3, s.Len())
	assert.Equal(t, []string{"a", "b", "c"}, s.Slice())
}

func TestAdd(t *testing.T) {
	s, _ := mapset.New(mapset.WithSorted(intCmp))
	added := s.Add(1, 2, 3)
	assert.Equal(t, 3, added)
	assert.Equal(t, 3, s.Len())
	assert.Equal(t, []int{1, 2, 3}, s.Slice())
}

func TestRemove(t *testing.T) {
	s, _ := mapset.New[int](mapset.WithSorted(intCmp))
	s.Add(1, 2, 3)
	assert.True(t, s.ContainsOne(2))
	assert.True(t, s.Contains(2))
	assert.Equal(t, []int{1, 2, 3}, s.Slice())
	s.Remove(2)
	assert.Equal(t, 2, s.Len())
	assert.False(t, s.ContainsOne(2))
	assert.False(t, s.Contains(2))
	assert.Equal(t, []int{1, 3}, s.Slice())
}

func TestPop(t *testing.T) {
	s, _ := mapset.New[int]()
	s.Add(1, 2, 3)
	elem, ok := s.Pop()
	assert.True(t, ok)
	assert.Contains(t, []int{1, 2, 3}, elem)
	assert.Equal(t, 2, s.Len())
}

func TestClear(t *testing.T) {
	s, _ := mapset.New[int]()
	s.Add(1, 2, 3)
	s.Clear()
	assert.True(t, s.IsEmpty())
	assert.Equal(t, 0, s.Len())
}

func TestContains(t *testing.T) {
	s, _ := mapset.New[int]()
	s.Add(1, 2, 3)
	assert.True(t, s.Contains(1))
	assert.True(t, s.Contains(1, 2))
	assert.True(t, s.Contains(1, 2, 3))
	assert.False(t, s.Contains(1, 2, 3, 4))
	assert.False(t, s.Contains(4))
}

func TestContainsOne(t *testing.T) {
	s, _ := mapset.New[int]()
	s.Add(1, 2, 3)
	assert.True(t, s.ContainsOne(1))
	assert.True(t, s.ContainsOne(2))
	assert.True(t, s.ContainsOne(3))
	assert.False(t, s.ContainsOne(4))
}

func TestContainsAny(t *testing.T) {
	s, _ := mapset.New[int]()
	s.Add(1, 2, 3)
	assert.True(t, s.ContainsAny(1))
	assert.True(t, s.ContainsAny(1, 2))
	assert.True(t, s.ContainsAny(1, 2, 3))
	assert.True(t, s.ContainsAny(1, 2, 3, 4))
	assert.False(t, s.Contains(4))
}

func TestEqual(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s2, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s3, _ := mapset.NewFromSlice([]int{4, 5, 6})
	assert.True(t, s1.Equal(s2))
	assert.False(t, s1.Equal(s3))
}

func TestRange(t *testing.T) {
	s, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSorted(intCmp))

	elements := make([]int, 0)
	count := 0
	s.Range(func(e int) bool {
		elements = append(elements, e)
		count++
		return true
	})

	assert.Equal(t, 3, count)
	assert.Equal(t, []int{1, 2, 3}, elements)
}

func TestIsEmpty(t *testing.T) {
	s, _ := mapset.New[int]()
	assert.True(t, s.IsEmpty())
	s.Add(1)
	assert.False(t, s.IsEmpty())
}

func TestIter(t *testing.T) {
	t.Run("iterate all", func(t *testing.T) {
		s, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSorted(intCmp))
		elements := make([]int, 0, 3)
		for e := range s.Iter() {
			elements = append(elements, e)
		}
		assert.Equal(t, []int{1, 2, 3}, elements)
	})

	// An early break must not leave the read lock held and deadlock later writers
	// on a concurrent-safe set.
	t.Run("early break does not block writers", func(t *testing.T) {
		s, _ := mapset.New(mapset.WithSafe[int]())
		s.Add(1, 2, 3, 4, 5)
		for range s.Iter() {
			break
		}

		done := make(chan struct{})
		go func() {
			s.Add(6)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Add blocked: Iter still holds the read lock after an early break")
		}
	})
}

func TestSeq(t *testing.T) {
	t.Run("iterate all", func(t *testing.T) {
		s, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSorted(intCmp))
		elements := make([]int, 0, 3)
		for e := range s.Seq() {
			elements = append(elements, e)
		}
		assert.Equal(t, []int{1, 2, 3}, elements)
	})

	t.Run("early break stops iteration", func(t *testing.T) {
		s, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSorted(intCmp))
		var first int
		for e := range s.Seq() {
			first = e
			break
		}
		assert.Equal(t, 1, first)
	})
}

func TestIsSubset(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2})
	s2, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s3, _ := mapset.NewFromSlice([]int{1, 2, 3})
	assert.True(t, s1.IsSubset(s2))
	assert.True(t, s2.IsSubset(s3))
	assert.False(t, s2.IsSubset(s1))
}

func TestIsProperSubset(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2})
	s2, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s3, _ := mapset.NewFromSlice([]int{1, 2, 3})
	assert.True(t, s1.IsProperSubset(s2))
	assert.False(t, s2.IsProperSubset(s3))
	assert.False(t, s2.IsProperSubset(s1))
}

func TestIsSuperset(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s2, _ := mapset.NewFromSlice([]int{1, 2})
	s3, _ := mapset.NewFromSlice([]int{1, 2})
	assert.True(t, s1.IsSuperset(s2))
	assert.True(t, s2.IsSuperset(s3))
	assert.False(t, s2.IsSuperset(s1))
}

func TestIsProperSuperset(t *testing.T) {
	t.Run("proper superset", func(t *testing.T) {
		s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
		s2, _ := mapset.NewFromSlice([]int{1, 2})
		s3, _ := mapset.NewFromSlice([]int{1, 2})
		assert.True(t, s1.IsProperSuperset(s2))
		assert.False(t, s2.IsProperSuperset(s3))
		assert.False(t, s2.IsProperSuperset(s1))
	})

	t.Run("nil", func(t *testing.T) {
		empty, _ := mapset.New[int]()
		nonEmpty, _ := mapset.NewFromSlice([]int{1})
		require.NotPanics(t, func() {
			assert.False(t, empty.IsProperSuperset(nil))
			assert.True(t, nonEmpty.IsProperSuperset(nil))
		})
	})
}

func TestDifference(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSorted(intCmp))
	s2, _ := mapset.NewFromSlice([]int{2, 3, 4}, mapset.WithSorted(intCmp))
	diff1 := s1.Difference(s2)
	diff2 := s2.Difference(s1)
	assert.Equal(t, []int{1}, diff1.Slice())
	assert.Equal(t, []int{4}, diff2.Slice())
}

func TestSymmetricDifference(t *testing.T) {
	t.Run("unsafe", func(t *testing.T) {
		s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
		s2, _ := mapset.NewFromSlice([]int{2, 3, 4})
		diff1 := s1.SymmetricDifference(s2)
		diff2 := s2.SymmetricDifference(s1)
		assert.True(t, diff1.Contains(1, 4))
		assert.True(t, diff2.Contains(1, 4))
	})

	t.Run("safe", func(t *testing.T) {
		s1, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSafe[int]())
		s2, _ := mapset.NewFromSlice([]int{2, 3, 4}, mapset.WithSafe[int]())
		diff := s1.SymmetricDifference(s2)
		assert.Equal(t, 2, diff.Len())
		assert.True(t, diff.Contains(1, 4))
	})

	t.Run("concurrent", func(t *testing.T) {
		runConcurrentBinaryOp(t, func(a, b *mapset.Set[int]) { _ = a.SymmetricDifference(b) })
	})
}

func TestUnion(t *testing.T) {
	t.Run("union", func(t *testing.T) {
		s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
		s2, _ := mapset.NewFromSlice([]int{3, 4, 5})
		union := s1.Union(s2)
		assert.Equal(t, 5, union.Len())
		assert.True(t, union.Contains(1, 2, 3, 4, 5))
	})

	t.Run("concurrent", func(t *testing.T) {
		runConcurrentBinaryOp(t, func(a, b *mapset.Set[int]) { _ = a.Union(b) })
	})
}

func TestIntersect(t *testing.T) {
	t.Run("intersect", func(t *testing.T) {
		s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
		s2, _ := mapset.NewFromSlice([]int{2, 3, 4})
		intersect := s1.Intersect(s2)
		assert.Equal(t, 2, intersect.Len())
		assert.True(t, intersect.Contains(2, 3))
	})

	t.Run("nil", func(t *testing.T) {
		s, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSorted(intCmp))
		var got *mapset.Set[int]
		require.NotPanics(t, func() {
			got = s.Intersect(nil)
		})
		assert.Equal(t, 0, got.Len())
		assert.Equal(t, []int{}, got.Slice())
	})

	t.Run("concurrent", func(t *testing.T) {
		runConcurrentBinaryOp(t, func(a, b *mapset.Set[int]) { _ = a.Intersect(b) })
	})
}

func TestClone(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s2 := s1.Clone()
	assert.True(t, s1.Equal(s2))
	s1.Add(4)
	assert.False(t, s1.Equal(s2))
	s2.Add(4)
	assert.True(t, s1.Equal(s2))
}

func TestSlice(t *testing.T) {
	s, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSorted(intCmp))
	assert.Equal(t, []int{1, 2, 3}, s.Slice())
}

func TestMarshalJSON(t *testing.T) {
	s, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSorted(intCmp))
	data, err := s.MarshalJSON()
	require.NoError(t, err)
	assert.JSONEq(t, "[1,2,3]", string(data))
}

func TestUnmarshalJSON(t *testing.T) {
	data := []byte("[1,2,3]")
	var s mapset.Set[int]
	err := s.UnmarshalJSON(data)
	require.NoError(t, err)
	assert.True(t, s.Contains(1, 2, 3))
}

// runConcurrentBinaryOp runs op in both directions against two concurrent-safe
// sets while other goroutines mutate them. Run with -race it flags any map read
// that is not guarded by the other set's lock, and any lock-ordering deadlock
// surfaces as a hang.
func runConcurrentBinaryOp(t *testing.T, op func(a, b *mapset.Set[int])) {
	t.Helper()
	a, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSafe[int]())
	b, _ := mapset.NewFromSlice([]int{3, 4, 5}, mapset.WithSafe[int]())

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(4)
		go func() { defer wg.Done(); op(a, b) }()
		go func() { defer wg.Done(); op(b, a) }()
		go func() { defer wg.Done(); a.Add(i) }()
		go func() { defer wg.Done(); b.Remove(i) }()
	}
	wg.Wait()
}
