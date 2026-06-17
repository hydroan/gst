package circularbuffer

import (
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/ds/types"
)

// CircularBuffer represents a generic circular buffer implementation.
// It supports thread-safe operations and can be configured to either drop new data
// or overwrite old data when the buffer is full.
type CircularBuffer[E any] struct {
	buf     []E
	head    int
	tail    int
	size    int
	maxSize int

	safe bool
	mu   types.Locker
	drop bool
}

// New creates a new circular buffer with the specified size.
func New[E any](size int, ops ...Option[E]) (*CircularBuffer[E], error) {
	if size <= 0 {
		return nil, errors.New("size must be greater than 0")
	}
	cb := &CircularBuffer[E]{maxSize: size, mu: types.FakeLocker{}}

	for _, op := range ops {
		if op == nil {
			continue
		}
		if err := op(cb); err != nil {
			return nil, err
		}
	}

	return cb, nil
}

// NewFromSlice creates a new circular buffer initialized with the given slice.
// The buffer size will be equal to the length of the slice and will contain
// all elements from the slice.
func NewFromSlice[E any](slice []E, ops ...Option[E]) (*CircularBuffer[E], error) {
	cb, err := New(len(slice), ops...)
	if err != nil {
		return nil, err
	}
	for _, e := range slice {
		cb.Enqueue(e)
	}
	return cb, nil
}

// Enqueue adds an element to the CircularBuffer.
// Returns false if the buffer is full, otherwise true.
func (cb *CircularBuffer[E]) Enqueue(e E) bool {
	if cb.safe {
		cb.mu.Lock()
		defer cb.mu.Unlock()
	}

	return cb.enqueue(e)
}

func (cb *CircularBuffer[E]) enqueue(e E) bool {
	if cb.buf == nil {
		cb.buf = make([]E, cb.maxSize)
	}
	if cb.size == cb.maxSize {
		if cb.drop {
			return false
		}
		cb.head = (cb.head + 1) % cb.maxSize
	} else {
		cb.size++
	}
	cb.buf[cb.tail] = e
	cb.tail = (cb.tail + 1) % cb.maxSize
	return true
}

// Dequeue removes an element from the CircularBuffer.
// Returns zero value and false if the buffer is empty, otherwise the element and true.
func (cb *CircularBuffer[E]) Dequeue() (E, bool) {
	if cb.safe {
		cb.mu.Lock()
		defer cb.mu.Unlock()
	}

	if cb.size == 0 {
		var e E
		return e, false
	}
	d := cb.buf[cb.head]
	var e E
	cb.buf[cb.head] = e
	cb.head = (cb.head + 1) % cb.maxSize
	cb.size--
	return d, true
}

// Peek returns the first element in the CircularBuffer without removing it.
// Returns zero value and false if the buffer is empty, otherwise the element and true.
func (cb *CircularBuffer[E]) Peek() (E, bool) {
	if cb.safe {
		cb.mu.RLock()
		defer cb.mu.RUnlock()
	}

	if cb.size == 0 {
		var e E
		return e, false
	}
	return cb.buf[cb.head], true
}

// IsEmpty reports whether the CircularBuffer is empty.
func (cb *CircularBuffer[E]) IsEmpty() bool {
	if cb.safe {
		cb.mu.RLock()
		defer cb.mu.RUnlock()
	}

	if cb.size == 0 {
		return true
	}
	return false
}

// IsFull reports whether the CircularBuffer is full.
func (cb *CircularBuffer[E]) IsFull() bool {
	if cb.safe {
		cb.mu.RLock()
		defer cb.mu.RUnlock()
	}

	if cb.size == 0 {
		return false
	}
	return true
}

// Len returns the number of elements in the CircularBuffer.
func (cb *CircularBuffer[E]) Len() int {
	if cb.safe {
		cb.mu.RLock()
		defer cb.mu.RUnlock()
	}

	return cb.size
}

// Clear removes all elements from the CircularBuffer.
func (cb *CircularBuffer[E]) Clear() {
	if cb.safe {
		cb.mu.Lock()
		defer cb.mu.Unlock()
	}

	if cb.size == 0 {
		return
	}
	var e E
	for i := range cb.buf {
		cb.buf[i] = e
	}
	cb.head = 0
	cb.tail = 0
	cb.size = 0
}

// Slice returns a slice of all elements in the CircularBuffer.
// It returns empty slice(not nil) if the CircularBuffer is empty.
func (cb *CircularBuffer[E]) Slice() []E {
	if cb.safe {
		cb.mu.RLock()
		defer cb.mu.RUnlock()
	}

	if cb.size == 0 {
		return make([]E, 0)
	}
	result := make([]E, cb.size)
	if cb.head == cb.tail {
		n := copy(result, cb.buf[cb.head:])
		copy(result[n:], cb.buf[:cb.tail])
		return result
	}
	if cb.head < cb.tail {
		copy(result, cb.buf[cb.head:cb.tail])
	} else {
		n := copy(result, cb.buf[cb.head:])
		copy(result[n:], cb.buf[:cb.tail])
	}
	return result
}

// Clone returns a copy of the CircularBuffer.
func (cb *CircularBuffer[E]) Clone() *CircularBuffer[E] {
	if cb.safe {
		cb.mu.RLock()
		defer cb.mu.RUnlock()
	}

	clone, _ := New(cb.maxSize, cb.options()...)
	idx := cb.head
	size := len(cb.buf)
	for range cb.size {
		clone.Enqueue(cb.buf[idx])
		idx = (idx + 1) % size
	}
	return clone
}

func (cb *CircularBuffer[E]) options() []Option[E] {
	ops := make([]Option[E], 0)

	if cb.safe {
		ops = append(ops, WithSafe[E]())
	}
	if cb.drop {
		ops = append(ops, WithDrop[E]())
	}
	return ops
}

// Range calls "fn" for each element in the CircularBuffer.
// function "fn" returns false to stop the iteration.
func (cb *CircularBuffer[E]) Range(fn func(e E) bool) {
	if fn == nil {
		return
	}
	if cb.safe {
		cb.mu.RLock()
		defer cb.mu.RUnlock()
	}

	idx := cb.head
	size := len(cb.buf)
	for range cb.size {
		if !fn(cb.buf[idx]) {
			return
		}
		idx = (idx + 1) % size
	}
}

// String returns a string representation of the CircularBuffer.
func (cb *CircularBuffer[E]) String() string {
	if cb.safe {
		cb.mu.RLock()
		defer cb.mu.RUnlock()
	}

	if cb.size == 0 {
		return "CircularBuffer (empty)"
	}
	values := make([]string, 0, cb.size)
	idx := cb.head
	size := len(cb.buf)
	for range cb.size {
		values = append(values, fmt.Sprintf("%v", cb.buf[idx]))
		idx = (idx + 1) % size
	}
	return fmt.Sprintf("CircularBuffer {%s}", strings.Join(values, ", "))
}
