package priorityqueue

import (
	"fmt"
	"strings"

	"github.com/hydroan/gst/ds/heap/binaryheap"
	"github.com/hydroan/gst/ds/types"
)

// Queue represents a priority queue.
// The queue is implemented by a binary heap.
type Queue[E any] struct {
	heap *binaryheap.Heap[E]
	cmp  func(E, E) int

	safe bool
	mu   types.Locker

	maxPriority bool
}

// New creates and returns a priority queue with the given comprarison function and options.
// The provided options can be used to configures the priority queue property.
// For example:
//   - WithSafe(): creates a thread-safe priority queue.
//   - WithMaxPriority(): creates a priority queue where higher value has higher priority.
//
// By defaults, it creates a non-thread-safe priority queue use min-heap ordering.
// (smaller values have higher priority).
func New[E any](cmp func(E, E) int, ops ...Option[E]) (*Queue[E], error) {
	if cmp == nil {
		return nil, types.ErrComparisonNil
	}
	q := &Queue[E]{cmp: cmp, mu: types.FakeLocker{}}
	for _, op := range ops {
		if op == nil {
			continue
		}
		if err := op(q); err != nil {
			return nil, err
		}
	}
	var heap *binaryheap.Heap[E]
	var err error
	if q.maxPriority {
		heap, err = binaryheap.New(cmp, binaryheap.WithMaxHeap[E]())
	} else {
		heap, err = binaryheap.New(cmp)
	}
	if err != nil {
		return nil, err
	}
	q.heap = heap
	return q, nil
}

// Enqueue adds an element to the queue.
func (q *Queue[E]) Enqueue(e E) {
	if q.safe {
		q.mu.Lock()
		defer q.mu.Unlock()
	}

	q.heap.Push(e)
}

// Dequeue removes and returns the first element from the queue.
// Returns zero value of element and false if queue is empty.
func (q *Queue[E]) Dequeue() (E, bool) {
	if q.safe {
		q.mu.Lock()
		defer q.mu.Unlock()
	}

	return q.heap.Pop()
}

// Peek returns first element of the queue without remove it.
// Returns zero value of element and false if queue is empty.
func (q *Queue[E]) Peek() (E, bool) {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}
	return q.heap.Peek()
}

// IsEmpty reports whether the queue has no element.
func (q *Queue[E]) IsEmpty() bool {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}
	return q.heap.IsEmpty()
}

// Len returns the number of elements currently in the queue.
func (q *Queue[E]) Len() int {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}
	return q.heap.Size()
}

// Values returns all elements in the queue.
func (q *Queue[E]) Values() []E {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}
	return q.heap.Values()
}

// Clear removes all elements from the queue.
func (q *Queue[E]) Clear() {
	if q.safe {
		q.mu.Lock()
		defer q.mu.Unlock()
	}
	q.heap.Clear()
}

// Clone returns a deep copy of the queue.
func (q *Queue[E]) Clone() *Queue[E] {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}
	clone, _ := New(q.cmp, q.options()...)
	for _, v := range q.heap.Values() {
		clone.heap.Push(v)
	}
	return clone
}

func (q *Queue[E]) options() []Option[E] {
	ops := make([]Option[E], 0)
	if q.safe {
		ops = append(ops, WithSafe[E]())
	}
	return ops
}

// String returns a string representation of the queue.
func (q *Queue[E]) String() string {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}

	items := make([]string, 0, q.heap.Size())
	q.heap.Range(func(e E) bool {
		items = append(items, fmt.Sprintf("%v", e))
		return true
	})
	return fmt.Sprintf("queue:{%s}", strings.Join(items, ", "))
}
