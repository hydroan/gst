package linkedlist_test

import (
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/hydroan/gst/ds/list/linkedlist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	l, err := linkedlist.New[int]()
	require.NoError(t, err)
	assert.NotNil(t, l)
	assert.Zero(t, l.Len())
	assert.True(t, l.IsEmpty())
}

func TestNewFromSlice(t *testing.T) {
	tests := []struct {
		name      string
		input     []int
		wantLen   int
		wantEmpty bool
	}{
		{"empty slice", []int{}, 0, true},
		{"single element", []int{1}, 1, false},
		{"multiple elements", []int{1, 2, 3}, 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := linkedlist.NewFromSlice(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantLen, l.Len())
			assert.Equal(t, tt.wantEmpty, l.IsEmpty())
			assert.Equal(t, tt.input, l.Slice())
		})
	}
}

func TestList_PushOperations(t *testing.T) {
	t.Run("PushBack", func(t *testing.T) {
		l, err := linkedlist.New[int]()
		require.NoError(t, err)
		n := l.PushBack(1)
		assert.Equal(t, 1, n.Value)
		assert.Equal(t, 1, l.Len())
		assert.Equal(t, []int{1}, l.Slice())

		n = l.PushBack(2)
		assert.Equal(t, 2, n.Value)
		assert.Equal(t, 2, l.Len())
		assert.Equal(t, []int{1, 2}, l.Slice())
	})

	t.Run("PushFront", func(t *testing.T) {
		l, err := linkedlist.New[int]()
		require.NoError(t, err)
		n := l.PushFront(1)
		assert.Equal(t, 1, n.Value)
		assert.Equal(t, 1, l.Len())
		assert.Equal(t, []int{1}, l.Slice())

		n = l.PushFront(2)
		assert.Equal(t, 2, n.Value)
		assert.Equal(t, 2, l.Len())
		assert.Equal(t, []int{2, 1}, l.Slice())
	})
}

func TestList_InsertOperations(t *testing.T) {
	t.Run("InsertAfter", func(t *testing.T) {
		l, err := linkedlist.New[int]()
		require.NoError(t, err)

		// Insert after nil node
		inserted := l.InsertAfter(nil, 1)
		assert.Nil(t, inserted)

		// Normal insert after
		l.PushBack(1)
		first := l.Find(1, func(a, b int) bool { return a == b })
		inserted = l.InsertAfter(first, 2)
		assert.NotNil(t, inserted)
		assert.Equal(t, []int{1, 2}, l.Slice())
	})

	t.Run("InsertBefore", func(t *testing.T) {
		l, err := linkedlist.New[int]()
		require.NoError(t, err)

		// Insert before nil node
		inserted := l.InsertBefore(nil, 1)
		assert.Nil(t, inserted)

		// Normal insert before
		l.PushBack(2)
		last := l.Find(2, func(a, b int) bool { return a == b })
		inserted = l.InsertBefore(last, 1)
		assert.NotNil(t, inserted)
		assert.Equal(t, []int{1, 2}, l.Slice())
	})
}

func TestList_RemoveOperations(t *testing.T) {
	t.Run("Remove", func(t *testing.T) {
		l, err := linkedlist.New[int]()
		require.NoError(t, err)

		// Remove from empty list
		l.Remove(nil)
		assert.Zero(t, l.Len())

		// Remove single element
		l.PushBack(1)
		node := l.Find(1, func(a, b int) bool { return a == b })
		l.Remove(node)
		assert.Zero(t, l.Len())
		assert.Empty(t, l.Slice())

		// Remove from multiple elements
		l.PushBack(1)
		l.PushBack(2)
		l.PushBack(3)
		node = l.Find(2, func(a, b int) bool { return a == b })
		l.Remove(node)
		assert.Equal(t, 2, l.Len())
		assert.Equal(t, []int{1, 3}, l.Slice())
	})

	t.Run("PopFront", func(t *testing.T) {
		l, err := linkedlist.New[int]()
		require.NoError(t, err)

		// Pop from empty list
		assert.Zero(t, l.PopFront())

		// Pop single element
		l.PushBack(1)
		v := l.PopFront()
		assert.Equal(t, 1, v)
		assert.Zero(t, l.Len())

		// Pop from multiple elements
		l.PushBack(1)
		l.PushBack(2)
		v = l.PopFront()
		assert.Equal(t, 1, v)
		assert.Equal(t, 1, l.Len())
		assert.Equal(t, []int{2}, l.Slice())
	})

	t.Run("PopBack", func(t *testing.T) {
		l, err := linkedlist.New[int]()
		require.NoError(t, err)

		// Pop from empty list
		assert.Zero(t, l.PopBack())

		// Pop single element
		l.PushBack(1)
		v := l.PopBack()
		assert.Equal(t, 1, v)
		assert.Zero(t, l.Len())

		// Pop from multiple elements
		l.PushBack(1)
		l.PushBack(2)
		v = l.PopBack()
		assert.Equal(t, 2, v)
		assert.Equal(t, 1, l.Len())
		assert.Equal(t, []int{1}, l.Slice())
	})
}

func TestList_Find(t *testing.T) {
	l, err := linkedlist.New[int]()
	require.NoError(t, err)

	// Find in empty list
	node := l.Find(1, func(a, b int) bool { return a == b })
	assert.Nil(t, node)

	// Find existing element
	l.PushBack(1)
	l.PushBack(2)
	l.PushBack(3)
	node = l.Find(2, func(a, b int) bool { return a == b })
	assert.NotNil(t, node)
	assert.Equal(t, 2, node.Value)

	// Find non-existing element
	node = l.Find(4, func(a, b int) bool { return a == b })
	assert.Nil(t, node)
}

func TestList_Reverse(t *testing.T) {
	tests := []struct {
		name  string
		input []int
		want  []int
	}{
		{"empty list", []int{}, []int{}},
		{"single element", []int{1}, []int{1}},
		{"two elements", []int{1, 2}, []int{2, 1}},
		{"multiple elements", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, []int{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := linkedlist.NewFromSlice(tt.input)
			require.NoError(t, err)
			l.Reverse()
			assert.Equal(t, tt.want, l.Slice())
		})
	}
}

func TestList_Merge(t *testing.T) {
	t.Run("merge with empty list", func(t *testing.T) {
		l1, err := linkedlist.NewFromSlice([]int{1, 3, 5})
		require.NoError(t, err)
		l2, err := linkedlist.New[int]()
		require.NoError(t, err)
		l1.Merge(l2)
		assert.Equal(t, []int{1, 3, 5}, l1.Slice())
	})

	t.Run("merge empty list", func(t *testing.T) {
		l1, err := linkedlist.New[int]()
		require.NoError(t, err)
		l2, err := linkedlist.NewFromSlice([]int{1, 3, 5})
		require.NoError(t, err)
		l1.Merge(l2)
		assert.Equal(t, []int{1, 3, 5}, l1.Slice())
	})

	t.Run("merge lists", func(t *testing.T) {
		l1, err := linkedlist.NewFromSlice([]int{1, 3, 5})
		require.NoError(t, err)
		l2, err := linkedlist.NewFromSlice([]int{2, 4, 6})
		require.NoError(t, err)
		l1.Merge(l2)
		assert.Equal(t, []int{1, 3, 5, 2, 4, 6}, l1.Slice())
	})
}

func TestList_MergeSorted(t *testing.T) {
	cmp := func(a, b int) int {
		//nolint:revive
		if a < b {
			return -1
		} else if a > b {
			return 1
		} else {
			return 0
		}
	}

	t.Run("merge with empty list", func(t *testing.T) {
		l1, err := linkedlist.NewFromSlice([]int{1, 3, 5})
		require.NoError(t, err)
		l2, err := linkedlist.New[int]()
		require.NoError(t, err)
		l1.MergeSorted(l2, cmp)
		assert.Equal(t, []int{1, 3, 5}, l1.Slice())
	})

	t.Run("merge empty list", func(t *testing.T) {
		l1, err := linkedlist.New[int]()
		require.NoError(t, err)
		l2, err := linkedlist.NewFromSlice([]int{1, 3, 5})
		require.NoError(t, err)
		l1.MergeSorted(l2, cmp)
		assert.Equal(t, []int{1, 3, 5}, l1.Slice())
	})

	t.Run("merge lists", func(t *testing.T) {
		l1, err := linkedlist.NewFromSlice([]int{1, 3, 5})
		require.NoError(t, err)
		l2, err := linkedlist.NewFromSlice([]int{2, 4, 6})
		require.NoError(t, err)
		l1.MergeSorted(l2, cmp)
		assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, l1.Slice())
	})

	t.Run("merge random lists", func(t *testing.T) {
		n := 10
		s1 := make([]int, 0, n)
		s2 := make([]int, 0, n)
		r1 := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano())))
		for range n {
			s1 = append(s1, r1.IntN(n))
		}
		time.Sleep(100 * time.Nanosecond)
		r2 := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano())))
		for range n {
			s2 = append(s2, r2.IntN(n))
		}

		l1, err := linkedlist.NewFromSlice(s1)
		require.NoError(t, err)
		l2, err := linkedlist.NewFromSlice(s2)
		require.NoError(t, err)
		l1.MergeSorted(l2, cmp)
		assert.True(t, slices.IsSorted(l1.Slice()))
	})
}

func TestList_Clone(t *testing.T) {
	original, err := linkedlist.NewFromSlice([]int{1, 2, 3})
	require.NoError(t, err)
	cloned, err := original.Clone()
	require.NoError(t, err)

	// Verify content is same
	assert.Equal(t, original.Slice(), cloned.Slice())

	// Verify it's a deep copy
	original.PushBack(4)
	assert.NotEqual(t, original.Slice(), cloned.Slice())
}

func TestList_Range(t *testing.T) {
	l, err := linkedlist.NewFromSlice([]int{1, 2, 3})
	require.NoError(t, err)
	var values []int
	l.Range(func(v int) bool {
		values = append(values, v)
		return true
	})
	assert.Equal(t, []int{1, 2, 3}, values)

	values = make([]int, 0)
	l, err = linkedlist.NewFromSlice([]int{1, 2, 3})
	require.NoError(t, err)
	l.Range(func(v int) bool {
		values = append(values, v)
		return false
	})
	assert.Equal(t, []int{1}, values)
}
