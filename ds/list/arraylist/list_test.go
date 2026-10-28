package arraylist_test

import (
	"sync"
	"testing"

	"github.com/hydroan/gst/ds/list/arraylist"
	"github.com/hydroan/gst/ds/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cmp(a, b int) int {
	return a - b
}

func TestNew(t *testing.T) {
	t.Run("valid comparison function", func(t *testing.T) {
		list, err := arraylist.New(cmp)
		require.NoError(t, err)
		assert.NotNil(t, list)
		assert.True(t, list.IsEmpty())
		assert.Equal(t, 0, list.Len())
	})

	t.Run("nil comparison function", func(t *testing.T) {
		list, err := arraylist.New[int](nil)
		require.ErrorIs(t, err, types.ErrComparisonNil)
		assert.Nil(t, list)
	})
}

func TestNewFromSlice(t *testing.T) {
	values := []int{1, 2, 3}
	list, err := arraylist.NewFromSlice(cmp, values)
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Equal(t, len(values), list.Len())
	assert.Equal(t, values, list.Values())
}

func TestAppend(t *testing.T) {
	list, _ := arraylist.New(cmp)

	list.Append(1, 2, 3)
	assert.Equal(t, 3, list.Len())
	assert.Equal(t, []int{1, 2, 3}, list.Values())
}

func TestInsert(t *testing.T) {
	list, _ := arraylist.New(cmp)

	list.Append(1, 3)
	list.Insert(1, 2)
	assert.Equal(t, []int{1, 2, 3}, list.Values())

	// Test inserting at the end
	list.Insert(3, 4)
	assert.Equal(t, []int{1, 2, 3, 4}, list.Values())

	// Test inserting out of range (no-op)
	list.Insert(10, 5)
	assert.Equal(t, []int{1, 2, 3, 4}, list.Values())
}

func TestSet(t *testing.T) {
	list, _ := arraylist.New(cmp)

	list.Append(1, 2, 3)
	list.Set(1, 5)
	assert.Equal(t, []int{1, 5, 3}, list.Values())

	// Test setting out of range (no-op)
	list.Set(10, 0)
	assert.Equal(t, []int{1, 5, 3}, list.Values())
}

func TestRemove(t *testing.T) {
	list, _ := arraylist.New(cmp)

	list.Append(1, 2, 2, 2, 2, 2, 2, 2, 2, 2, 3)
	list.Remove(2)
	assert.Equal(t, []int{1, 3}, list.Values())
}

func TestRemoveAt(t *testing.T) {
	list, _ := arraylist.New(cmp)

	list.Append(1, 2, 3)
	removed := list.RemoveAt(1)
	assert.Equal(t, 2, removed)
	assert.Equal(t, []int{1, 3}, list.Values())

	// Test removing out of range (no-op)
	removed = list.RemoveAt(10)
	assert.Zero(t, removed)
	assert.Equal(t, []int{1, 3}, list.Values())
}

func TestContains(t *testing.T) {
	list, _ := arraylist.New(cmp)

	list.Append(1, 2, 3)
	assert.True(t, list.Contains(1, 2))
	assert.True(t, list.Contains(1))
	assert.False(t, list.Contains(1, 4))
	assert.False(t, list.Contains(4))
	// All of nothing is present.
	assert.True(t, list.Contains())
}

func TestClear(t *testing.T) {
	list, _ := arraylist.New(cmp)

	list.Append(1, 2, 3)
	list.Clear()
	assert.True(t, list.IsEmpty())
	assert.Equal(t, 0, list.Len())
}

func TestSort(t *testing.T) {
	list, _ := arraylist.New(cmp)

	list.Append(3, 1, 2)
	list.Sort()
	assert.Equal(t, []int{1, 2, 3}, list.Values())
}

func TestLenIsEmpty(t *testing.T) {
	list, _ := arraylist.New(cmp)

	assert.True(t, list.IsEmpty())
	assert.Equal(t, 0, list.Len())

	list.Append(1)
	assert.False(t, list.IsEmpty())
	assert.Equal(t, 1, list.Len())
}

// TestNewOrdered pins the lists that compare their elements by their order,
// cmp.Compare: sorting orders them and IndexOf finds them by it, from an
// empty list or a slice alike; WithSafe gives one that takes concurrent
// appends.
func TestNewOrdered(t *testing.T) {
	l, err := arraylist.NewOrdered[int]()
	require.NoError(t, err)
	l.Append(3, 1, 2)
	l.Sort()
	assert.Equal(t, []int{1, 2, 3}, l.Values())
	assert.Equal(t, 2, l.IndexOf(3))

	words, err := arraylist.NewFromOrderedSlice([]string{"b", "c", "a"})
	require.NoError(t, err)
	words.Sort()
	assert.Equal(t, []string{"a", "b", "c"}, words.Values())

	safe, err := arraylist.NewOrdered(arraylist.WithSafe[int]())
	require.NoError(t, err)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() { safe.Append(i) })
	}
	wg.Wait()
	assert.Equal(t, 8, safe.Len())
}

// TestGetIndexOfSwapRange pins the reads and the swap by index: Get and
// IndexOf report an index out of range and an element that is not there,
// Swap leaves the list alone for an index out of range, and Range stops
// where its function returns false and does nothing for a nil one.
func TestGetIndexOfSwapRange(t *testing.T) {
	l, err := arraylist.NewFromSlice(cmp, []int{10, 20, 30})
	require.NoError(t, err)

	v, ok := l.Get(1)
	assert.True(t, ok)
	assert.Equal(t, 20, v)
	v, ok = l.Get(3)
	assert.False(t, ok)
	assert.Zero(t, v)
	_, ok = l.Get(-1)
	assert.False(t, ok)

	assert.Equal(t, 2, l.IndexOf(30))
	assert.Equal(t, -1, l.IndexOf(40))

	l.Swap(0, 2)
	assert.Equal(t, []int{30, 20, 10}, l.Values())
	l.Swap(0, 3)
	assert.Equal(t, []int{30, 20, 10}, l.Values())

	var seen []int
	l.Range(func(e int) bool {
		seen = append(seen, e)
		return e != 20
	})
	assert.Equal(t, []int{30, 20}, seen)
	l.Range(nil)
}
