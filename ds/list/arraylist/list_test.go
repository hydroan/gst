package arraylist_test

import (
	"testing"

	"github.com/hydroan/gst/ds/list/arraylist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cmp(a, b int) int {
	return a - b
}

func TestNew(t *testing.T) {
	list, err := arraylist.New(cmp)
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.True(t, list.IsEmpty())
	assert.Equal(t, 0, list.Len())
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
