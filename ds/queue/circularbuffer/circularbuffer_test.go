package circularbuffer_test

import (
	"encoding/json"
	"testing"

	"github.com/hydroan/gst/ds/queue/circularbuffer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircularBuffer_New(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		ops     []circularbuffer.Option[int]
		wantErr bool
	}{
		{
			name:    "valid size",
			size:    5,
			wantErr: false,
		},
		{
			name:    "zero size",
			size:    0,
			wantErr: true,
		},
		{
			name:    "negative size",
			size:    -1,
			wantErr: true,
		},
		{
			name:    "with safe option",
			size:    5,
			ops:     []circularbuffer.Option[int]{circularbuffer.WithSafe[int]()},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb, err := circularbuffer.New(tt.size, tt.ops...)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, cb)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cb)
			}
		})
	}
}

func TestCircularBuffer_EnqueueDequeue(t *testing.T) {
	t.Run("basic enqueue dequeue", func(t *testing.T) {
		cb, err := circularbuffer.New[int](3)
		require.NoError(t, err)

		// Test Enqueue
		assert.True(t, cb.Enqueue(1))
		assert.True(t, cb.Enqueue(2))
		assert.True(t, cb.Enqueue(3))
		assert.Equal(t, 3, cb.Len())

		// Test Dequeue
		val, ok := cb.Dequeue()
		assert.True(t, ok)
		assert.Equal(t, 1, val)
		assert.Equal(t, 2, cb.Len())
	})

	t.Run("drop mode when full", func(t *testing.T) {
		cb, err := circularbuffer.New(2, circularbuffer.WithDrop[int]())
		require.NoError(t, err)

		assert.True(t, cb.Enqueue(1))
		assert.True(t, cb.Enqueue(2))
		assert.False(t, cb.Enqueue(3)) // Should be dropped

		val, ok := cb.Dequeue()
		assert.True(t, ok)
		assert.Equal(t, 1, val)
	})

	t.Run("overwrite mode when full", func(t *testing.T) {
		cb, err := circularbuffer.New[int](2)
		require.NoError(t, err)

		assert.True(t, cb.Enqueue(1))
		assert.True(t, cb.Enqueue(2))
		assert.True(t, cb.Enqueue(3)) // Should overwrite 1

		val, ok := cb.Dequeue()
		assert.True(t, ok)
		assert.Equal(t, 2, val)
	})

	t.Run("dequeue empty buffer", func(t *testing.T) {
		cb, err := circularbuffer.New[int](3)
		require.NoError(t, err)

		val, ok := cb.Dequeue()
		assert.False(t, ok)
		assert.Zero(t, val)
	})
}

func TestCircularBuffer_Peek(t *testing.T) {
	t.Run("peek non-empty buffer", func(t *testing.T) {
		cb, _ := circularbuffer.New[int](3)
		cb.Enqueue(1)
		cb.Enqueue(2)

		val, ok := cb.Peek()
		assert.True(t, ok)
		assert.Equal(t, 1, val)
		assert.Equal(t, 2, cb.Len()) // Ensure peek didn't remove element
	})

	t.Run("peek empty buffer", func(t *testing.T) {
		cb, _ := circularbuffer.New[int](3)

		val, ok := cb.Peek()
		assert.False(t, ok)
		assert.Zero(t, val)
	})
}

func TestCircularBuffer_Clear(t *testing.T) {
	cb, _ := circularbuffer.New[int](3)
	cb.Enqueue(1)
	cb.Enqueue(2)

	cb.Clear()
	assert.True(t, cb.IsEmpty())
	assert.Equal(t, 0, cb.Len())

	// Test that we can still use the buffer after clearing
	assert.True(t, cb.Enqueue(3))
	val, ok := cb.Peek()
	assert.True(t, ok)
	assert.Equal(t, 3, val)
}

func TestCircularBuffer_Clone(t *testing.T) {
	t.Run("clone non-empty buffer", func(t *testing.T) {
		cb, _ := circularbuffer.New[int](3)
		cb.Enqueue(1)
		cb.Enqueue(2)

		clone := cb.Clone()
		assert.Equal(t, cb.Len(), clone.Len())

		// Verify elements are the same
		cbSlice := cb.Slice()
		cloneSlice := clone.Slice()
		assert.Equal(t, cbSlice, cloneSlice)

		// Verify modifications don't affect original
		clone.Enqueue(3)
		assert.NotEqual(t, cb.Len(), clone.Len())
	})

	t.Run("clone empty buffer", func(t *testing.T) {
		cb, _ := circularbuffer.New[int](3)
		clone := cb.Clone()
		assert.True(t, clone.IsEmpty())
	})
}

func TestCircularBuffer_Range(t *testing.T) {
	cb, _ := circularbuffer.New[int](5)
	numbers := []int{1, 2, 3, 4}
	for _, n := range numbers {
		cb.Enqueue(n)
	}

	t.Run("range all elements", func(t *testing.T) {
		var result []int
		cb.Range(func(e int) bool {
			result = append(result, e)
			return true
		})
		assert.Equal(t, numbers, result)
	})

	t.Run("range with early stop", func(t *testing.T) {
		var result []int
		cb.Range(func(e int) bool {
			if e > 2 {
				return false
			}
			result = append(result, e)
			return true
		})
		assert.Equal(t, []int{1, 2}, result)
	})
}

func TestCircularBuffer_Slice(t *testing.T) {
	t.Run("slice with wrap-around", func(t *testing.T) {
		cb, _ := circularbuffer.New[int](3)
		cb.Enqueue(1)
		cb.Enqueue(2)
		cb.Enqueue(3)
		cb.Dequeue()  // Remove 1
		cb.Enqueue(4) // This should wrap around

		slice := cb.Slice()
		assert.Equal(t, []int{2, 3, 4}, slice)
	})

	t.Run("slice empty buffer", func(t *testing.T) {
		cb, _ := circularbuffer.New[int](3)
		slice := cb.Slice()
		assert.Empty(t, slice)
		assert.NotNil(t, slice)
	})
}

func TestCircularBuffer_Encoding(t *testing.T) {
	var err error
	var cb, cb2, cb3 *circularbuffer.CircularBuffer[int]
	if cb, err = circularbuffer.New(3, circularbuffer.WithSafe[int]()); err != nil {
		t.Fatal(err)
	}
	if cb2, err = circularbuffer.New(2, circularbuffer.WithSafe[int]()); err != nil {
		t.Fatal(err)
	}
	if cb3, err = circularbuffer.New(2, circularbuffer.WithSafe[int](), circularbuffer.WithDrop[int]()); err != nil {
		t.Fatal(err)
	}
	cb.Enqueue(1)
	cb.Enqueue(2)
	cb.Enqueue(3)

	b, err := json.Marshal(cb)
	if err != nil {
		t.Fatal(err)
	}

	if err = json.Unmarshal(b, cb2); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, cb3); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, []int{1, 2, 3}, cb.Slice())
	assert.Equal(t, []int{2, 3}, cb2.Slice())
	assert.Equal(t, []int{1, 2}, cb3.Slice())
}
