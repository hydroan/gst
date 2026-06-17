package binaryheap_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hydroan/gst/ds/heap/binaryheap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createHeap(t *testing.T) *binaryheap.Heap[int] {
	t.Helper()
	h, err := binaryheap.NewOrdered[int](binaryheap.WithSafe[int]())
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestBinaryHeap_New(t *testing.T) {
	h, err := binaryheap.NewOrdered(binaryheap.WithSafe[int]())
	require.NoError(t, err)
	assert.NotNil(t, h)

	// Test empty heap
	assert.True(t, h.IsEmpty())
	assert.Equal(t, 0, h.Size())
	_, ok := h.Peek()
	assert.False(t, ok)
	_, ok = h.Pop()
	assert.False(t, ok)

	slice := []int{6, 5, 4, 3, 2, 1}
	h, err = binaryheap.NewFromOrderedSlice(slice)
	require.NoError(t, err)
	assert.Equal(t, len(slice), h.Size())
	assert.False(t, h.IsEmpty())
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, h.Values())
	// fmt.Println(h.String())

	m := map[int]string{
		2: "b",
		5: "e",
		1: "a",
		3: "c",
		4: "d",
		6: "f",
	}

	h, err = binaryheap.NewFromOrderedMapKeys(m, binaryheap.WithSafe[int]())
	require.NoError(t, err)
	assert.Equal(t, 6, h.Size())
	assert.False(t, h.IsEmpty())
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, h.Values())

	h2, err := binaryheap.NewFromOrderedMapValues(m, binaryheap.WithSafe[string]())
	require.NoError(t, err)
	assert.Equal(t, 6, h2.Size())
	assert.False(t, h2.IsEmpty())
	assert.Equal(t, []string{"a", "b", "c", "d", "e", "f"}, h2.Values())
}

func TestBinaryHeap_MaxHeap(t *testing.T) {
	slice := []int{5, 1, 3, 2, 4, 6}
	h, err := binaryheap.NewFromOrderedSlice(slice)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, h.Values())

	h, err = binaryheap.NewFromOrderedSlice(slice, binaryheap.WithMaxHeap[int]())
	require.NoError(t, err)
	assert.Equal(t, []int{6, 5, 4, 3, 2, 1}, h.Values())

	m := map[int]string{
		2: "b",
		5: "e",
		1: "a",
		3: "c",
		4: "d",
		6: "f",
	}
	h, err = binaryheap.NewFromOrderedMapKeys(m)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, h.Values())

	h2, err := binaryheap.NewFromOrderedMapValues(m, binaryheap.WithMaxHeap[string]())
	require.NoError(t, err)
	assert.Equal(t, []string{"f", "e", "d", "c", "b", "a"}, h2.Values())
}

func TestBinaryHeap_PushPop(t *testing.T) {
	h := createHeap(t)

	values := []int{5, 3, 7, 1, 4, 6, 2}
	for _, v := range values {
		h.Push(v)
	}
	assert.Equal(t, len(values), h.Size())
	assert.False(t, h.IsEmpty())

	val, ok := h.Peek()
	assert.True(t, ok)
	assert.Equal(t, 1, val)

	expected := []int{1, 2, 3, 4, 5, 6, 7}
	for _, exp := range expected {
		val, ok := h.Pop()
		assert.True(t, ok)
		assert.Equal(t, exp, val)
	}
	assert.True(t, h.IsEmpty())
}

func TestBinaryHeap_Values(t *testing.T) {
	h := createHeap(t)

	// Test empty heap
	assert.Empty(t, h.Values())

	// Test with values
	input := []int{5, 3, 7, 1, 4}
	for _, v := range input {
		h.Push(v)
	}

	values := h.Values()
	assert.Len(t, values, len(input))

	// Verify that Values() returns a copy
	originalValues := h.Values()
	h.Pop()
	newValues := h.Values()
	assert.NotEqual(t, len(originalValues), len(newValues))
}

func TestBinaryHeap_Range(t *testing.T) {
	h := createHeap(t)

	input := []int{5, 3, 7, 1, 4}
	for _, v := range input {
		h.Push(v)
	}

	sum := 0
	count := 0
	h.Range(func(e int) bool {
		sum += e
		count++
		return true
	})

	assert.Equal(t, len(input), count)
	assert.Equal(t, 20, sum) // 5+3+7+1+4 = 20

	count = 0
	h.Range(func(e int) bool {
		count++
		return count < 3
	})
	assert.Equal(t, 3, count)
}

func TestBinaryHeap_ErrorCases(t *testing.T) {
	// Test nil comparison function
	h, err := binaryheap.New[int](nil)
	require.Error(t, err)
	assert.Nil(t, h)

	// Test nil options
	h, err = binaryheap.New(func(a, b int) int { return a - b }, nil)
	require.NoError(t, err)
	assert.NotNil(t, h)
}

func TestBinaryHeap_String(t *testing.T) {
	h, err := binaryheap.NewFromOrderedSlice([]int{5, 3, 7, 1, 4, 8, 9, 2}, binaryheap.WithSafe[int]())
	require.NoError(t, err)
	fmt.Println(h)
	fmt.Println(h.Values())
}

func TestBInaryHeap_Encoding(t *testing.T) {
	h, err := binaryheap.NewFromOrderedSlice([]int{5, 3, 7, 1, 4, 8, 9, 2}, binaryheap.WithSafe[int]())
	require.NoError(t, err)

	bytesData, err := json.Marshal(h)
	require.NoError(t, err)

	h2, err := binaryheap.NewOrdered[int]()
	require.NoError(t, err)
	err = json.Unmarshal(bytesData, &h2)
	require.NoError(t, err)
	assert.Equal(t, h.Values(), h2.Values())
}
