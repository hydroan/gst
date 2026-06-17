package arrayqueue_test

import (
	"testing"

	"github.com/hydroan/gst/ds/queue/arrayqueue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intCmp(a, b int) int {
	return a - b
}

func stringCmp(a, b string) int {
	//nolint:revive
	if a < b {
		return -1
	} else if a > b {
		return 1
	} else {
		return 0
	}
}

func TestQueue(t *testing.T) {
	t.Run("New", func(t *testing.T) {
		m := map[int]string{1: "a", 2: "b", 3: "c"}

		q, err := arrayqueue.New(intCmp, arrayqueue.WithSafe[int]())
		require.NoError(t, err)
		assert.Equal(t, 0, q.Len())
		assert.True(t, q.IsEmpty())
		assert.Empty(t, q.Values())
		assert.Equal(t, "queue:{}", q.String())

		q, err = arrayqueue.NewFromSlice(intCmp, []int{1, 2, 3}, arrayqueue.WithSafe[int]())
		require.NoError(t, err)
		assert.Equal(t, 3, q.Len())
		assert.False(t, q.IsEmpty())
		assert.Equal(t, []int{1, 2, 3}, q.Values())
		assert.Equal(t, "queue:{1, 2, 3}", q.String())

		q, err = arrayqueue.NewFromMapKeys(intCmp, m, arrayqueue.WithSafe[int]())
		require.NoError(t, err)
		assert.NotNil(t, q)
		assert.False(t, q.IsEmpty())
		assert.Equal(t, 3, q.Len())
		assert.ElementsMatch(t, []int{1, 2, 3}, q.Values())

		q2, err := arrayqueue.NewFromMapValues(stringCmp, m, arrayqueue.WithSafe[string]())
		require.NoError(t, err)
		assert.NotNil(t, q2)
		assert.False(t, q2.IsEmpty())
		assert.Equal(t, 3, q2.Len())
		assert.ElementsMatch(t, []string{"a", "b", "c"}, q2.Values())
	})

	t.Run("Enqueue and Dequeue", func(t *testing.T) {
		q, err := arrayqueue.New(intCmp)
		require.NoError(t, err)
		assert.True(t, q.IsEmpty())

		q.Enqueue(1)
		q.Enqueue(2)
		q.Enqueue(3)
		assert.Equal(t, 3, q.Len())
		assert.False(t, q.IsEmpty())

		value, ok := q.Dequeue()
		assert.True(t, ok)
		assert.Equal(t, 1, value)
		assert.Equal(t, 2, q.Len())

		value, ok = q.Dequeue()
		assert.True(t, ok)
		assert.Equal(t, 2, value)

		value, ok = q.Dequeue()
		assert.True(t, ok)
		assert.Equal(t, 3, value)
		assert.True(t, q.IsEmpty())

		value, ok = q.Dequeue()
		assert.False(t, ok)
		assert.Zero(t, value)
	})

	t.Run("Peek", func(t *testing.T) {
		q, err := arrayqueue.New(intCmp)
		require.NoError(t, err)

		_, ok := q.Peek()
		assert.False(t, ok)

		q.Enqueue(10)
		value, ok := q.Peek()
		assert.True(t, ok)
		assert.Equal(t, 10, value)
		assert.Equal(t, 1, q.Len())
	})

	t.Run("Values", func(t *testing.T) {
		q, err := arrayqueue.New(intCmp)
		require.NoError(t, err)

		q.Enqueue(1)
		q.Enqueue(2)
		q.Enqueue(3)
		values := q.Values()
		assert.Equal(t, []int{1, 2, 3}, values)
	})

	t.Run("Clear", func(t *testing.T) {
		q, err := arrayqueue.New(intCmp)
		require.NoError(t, err)

		q.Enqueue(1)
		q.Enqueue(2)
		assert.False(t, q.IsEmpty())

		q.Clear()
		assert.True(t, q.IsEmpty())
		assert.Equal(t, 0, q.Len())
	})

	t.Run("Clone", func(t *testing.T) {
		q, err := arrayqueue.New(intCmp)
		require.NoError(t, err)

		q.Enqueue(1)
		q.Enqueue(2)
		clone := q.Clone()
		assert.Equal(t, q.Values(), clone.Values())

		q.Enqueue(3)
		assert.NotEqual(t, q.Values(), clone.Values())
	})
}

func TestQueueBoundaryCases(t *testing.T) {
	t.Run("Empty Queue Dequeue and Peek", func(t *testing.T) {
		q, err := arrayqueue.New(intCmp)
		require.NoError(t, err)

		value, ok := q.Dequeue()
		assert.False(t, ok)
		assert.Zero(t, value)

		value, ok = q.Peek()
		assert.False(t, ok)
		assert.Zero(t, value)
	})

	t.Run("Queue With One Element", func(t *testing.T) {
		q, err := arrayqueue.New(intCmp)
		require.NoError(t, err)

		q.Enqueue(42)
		assert.Equal(t, 1, q.Len())

		value, ok := q.Peek()
		assert.True(t, ok)
		assert.Equal(t, 42, value)

		value, ok = q.Dequeue()
		assert.True(t, ok)
		assert.Equal(t, 42, value)
		assert.True(t, q.IsEmpty())
	})
}
