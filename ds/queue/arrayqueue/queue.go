package arrayqueue

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hydroan/gst/ds/types"
)

// Queue represents a queue backed by a slice.
// This queue implements the FIFO (first-in, first-out) behavior.
type Queue[E any] struct {
	elements []E
	mu       types.Locker
	safe     bool
}

// New creates and initializes a empty queue.
// Options can be provided to customize the queue's properties (e.g., thread safety).
func New[E any](ops ...Option[E]) (*Queue[E], error) {
	q := &Queue[E]{elements: make([]E, 0), mu: types.FakeLocker{}}
	for _, op := range ops {
		if op == nil {
			continue
		}
		if err := op(q); err != nil {
			return nil, err
		}
	}
	return q, nil
}

// NewFromSlice creates and initializes a queue from the provided slice.
// Options can be provided to customize the queue's properties (e.g., thread safety).
func NewFromSlice[E any](slice []E, ops ...Option[E]) (*Queue[E], error) {
	q, err := New(ops...)
	if err != nil {
		return nil, err
	}
	q.elements = append(q.elements, slice...)
	return q, nil
}

// NewFromMapKeys creates and initializes a queue from the provided map keys.
// Options can be provided to customize the queue's properties (e.g., thread safety).
func NewFromMapKeys[K comparable, V any](m map[K]V, ops ...Option[K]) (*Queue[K], error) {
	q, err := New(ops...)
	if err != nil {
		return nil, err
	}
	for k := range m {
		q.elements = append(q.elements, k)
	}
	return q, nil
}

// NewFromMapValues creates and initializes a queue from the provided map values.
// Options can be provided to customize the queue's properties (e.g., thread safety).
func NewFromMapValues[K comparable, V any](m map[K]V, ops ...Option[V]) (*Queue[V], error) {
	q, err := New(ops...)
	if err != nil {
		return nil, err
	}
	for _, v := range m {
		q.elements = append(q.elements, v)
	}
	return q, nil
}

// Enqueue adds a element to the end of the queue.
func (q *Queue[E]) Enqueue(e E) {
	if q.safe {
		q.mu.Lock()
		defer q.mu.Unlock()
	}

	q.elements = append(q.elements, e)
}

// Dequeue removes first element of the queue.
// Returns zero value of element and false if queue is empty.
func (q *Queue[E]) Dequeue() (E, bool) {
	if q.safe {
		q.mu.Lock()
		defer q.mu.Unlock()
	}

	var zero E
	if len(q.elements) == 0 {
		return zero, false
	}
	e := q.elements[0]
	// Clearing the slot lets the backing array drop its reference to e.
	q.elements[0] = zero
	q.elements = q.elements[1:]
	return e, true
}

// Peek returns first element of the queue without remove it.
// Returns zero value of element and false if queue is empty.
func (q *Queue[E]) Peek() (E, bool) {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}

	if len(q.elements) == 0 {
		var zero E
		return zero, false
	}
	return q.elements[0], true
}

// IsEmpty reports whether the queue has no element.
func (q *Queue[E]) IsEmpty() bool {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}

	return len(q.elements) == 0
}

// Len returns the number of elements currently in the queue.
func (q *Queue[E]) Len() int {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}

	return len(q.elements)
}

// Values returns all elements in the queue in FIFO(first-in, first-out) order.
func (q *Queue[E]) Values() []E {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}

	return slices.Clone(q.elements)
}

// Clear removes all elements from the queue.
func (q *Queue[E]) Clear() {
	if q.safe {
		q.mu.Lock()
		defer q.mu.Unlock()
	}

	q.elements = make([]E, 0)
}

// Clone returns a deep copy of the queue.
func (q *Queue[E]) Clone() *Queue[E] {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}

	clone, _ := NewFromSlice(q.elements, q.options()...)
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

	items := make([]string, 0, len(q.elements))
	for _, e := range q.elements {
		items = append(items, fmt.Sprintf("%v", e))
	}
	return fmt.Sprintf("queue:{%s}", strings.Join(items, ", "))
}
