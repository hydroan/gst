package linkedstack_test

import (
	"testing"

	"github.com/hydroan/gst/ds/stack/linkedstack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newIntStack() (*linkedstack.Stack[int], error) {
	return linkedstack.New[int]()
}

func TestNew(t *testing.T) {
	s, err := linkedstack.New[int]()
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, 0, s.Len())
	assert.True(t, s.IsEmpty())
}

func TestNewFromSlice(t *testing.T) {
	s, err := linkedstack.NewFromSlice([]int{1, 2, 3}, linkedstack.WithSafe[int]())
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, 3, s.Len())
	assert.False(t, s.IsEmpty())
	assert.Equal(t, []int{3, 2, 1}, s.Values())
	assert.Equal(t, "stack:{3, 2, 1}", s.String())
}

func TestNewFromMapKeys(t *testing.T) {
	m := map[int]string{1: "a", 2: "b", 3: "c"}

	stack, err := linkedstack.NewFromMapKeys(m)

	require.NoError(t, err)
	assert.ElementsMatch(t, []int{1, 2, 3}, stack.Values())
}

func TestNewFromMapValues(t *testing.T) {
	m := map[int]string{1: "a", 2: "b", 3: "c"}

	stack, err := linkedstack.NewFromMapValues(func(a, b string) int {
		if a < b {
			return -1
		} else if a > b {
			return 1
		}
		return 0
	}, m)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b", "c"}, stack.Values())
}

func TestPushPop(t *testing.T) {
	stack, err := newIntStack()
	require.NoError(t, err)

	stack.Push(10)
	stack.Push(20)
	stack.Push(30)

	assert.Equal(t, 3, stack.Len())

	e, ok := stack.Pop()
	assert.True(t, ok)
	assert.Equal(t, 30, e)

	e, ok = stack.Pop()
	assert.True(t, ok)
	assert.Equal(t, 20, e)

	assert.Equal(t, 1, stack.Len())

	e, ok = stack.Pop()
	assert.True(t, ok)
	assert.Equal(t, 10, e)

	assert.True(t, stack.IsEmpty())
}

func TestPeek(t *testing.T) {
	stack, err := newIntStack()
	require.NoError(t, err)

	_, ok := stack.Peek()
	assert.False(t, ok)

	stack.Push(10)
	stack.Push(20)
	stack.Push(30)

	e, ok := stack.Peek()
	assert.True(t, ok)
	assert.Equal(t, 30, e)

	assert.Equal(t, 3, stack.Len())
}

func TestLen(t *testing.T) {
	stack, err := newIntStack()
	require.NoError(t, err)

	assert.Equal(t, 0, stack.Len())

	stack.Push(10)
	stack.Push(20)
	stack.Push(30)
	assert.Equal(t, 3, stack.Len())

	// Pop one element and check the length again
	stack.Pop()
	assert.Equal(t, 2, stack.Len())
}

func TestValues(t *testing.T) {
	stack, err := newIntStack()
	require.NoError(t, err)

	// Values on an empty stack
	values := stack.Values()
	assert.Empty(t, values)

	// Push some elements and check values
	stack.Push(10)
	stack.Push(20)
	stack.Push(30)

	values = stack.Values()
	assert.Equal(t, []int{30, 20, 10}, values)
}

func TestClear(t *testing.T) {
	stack, err := newIntStack()
	require.NoError(t, err)

	// Push elements
	stack.Push(10)
	stack.Push(20)
	stack.Push(30)

	// Clear the stack
	stack.Clear()

	// Ensure the stack is empty
	assert.True(t, stack.IsEmpty())
	assert.Equal(t, 0, stack.Len())
}

func TestClone(t *testing.T) {
	stack, err := newIntStack()
	require.NoError(t, err)

	// Push elements to the original stack
	stack.Push(10)
	stack.Push(20)
	stack.Push(30)

	// Clone the stack
	clone := stack.Clone()

	// FIXME: why stack is equal clone?
	// assert.NotEqual(t, stack, clone)
	assert.Equal(t, stack.Values(), clone.Values())

	// Modify the original stack
	stack.Pop()

	// Ensure the clone is unchanged
	assert.Equal(t, []int{30, 20, 10}, clone.Values())
}

func TestMarshalJSON(t *testing.T) {
	stack, err := newIntStack()
	stack.Push(3)
	stack.Push(2)
	stack.Push(1)
	require.NoError(t, err)
	b, err := stack.MarshalJSON()
	require.NoError(t, err)

	assert.JSONEq(t, "[1,2,3]", string(b))
}

func TestUnmarshalJSON(t *testing.T) {
	data := []byte("[1,2,3]")
	stack, err := linkedstack.New[int]()
	require.NoError(t, err)
	require.NoError(t, stack.UnmarshalJSON(data))
	assert.Equal(t, []int{3, 2, 1}, stack.Values())
	assert.Equal(t, "stack:{3, 2, 1}", stack.String())
}
