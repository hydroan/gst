package linkedqueue

import (
	"fmt"
	"strings"

	"github.com/hydroan/gst/ds/list/linkedlist"
	"github.com/hydroan/gst/ds/types"
)

// Queue represents a queue based on an linkedlist-backed list.
// This queue implements the FIFO (first-in, first-out) behavior.
type Queue[E any] struct {
	list *linkedlist.List[E]
	cmp  func(E, E) int
	mu   types.Locker
	safe bool
}

// New creates and initializes a empty queue.
// The "cmp" function is used to compare elements for equality.
// Options can be provided to customize the queue's properties (e.g., thread safety).
func New[E any](cmp func(E, E) int, ops ...Option[E]) (*Queue[E], error) {
	q := &Queue[E]{mu: types.FakeLocker{}}
	q.cmp = cmp
	var err error
	for _, op := range ops {
		if op == nil {
			continue
		}
		if err = op(q); err != nil {
			return nil, err
		}
	}
	// internal list alway concurrent unsafe.
	q.list, err = linkedlist.New[E]()
	if err != nil {
		return nil, err
	}
	return q, nil
}

// NewFromSlice creates and initializes a queue from the provided slice.
// The "cmp" function is used to compare elements for equality.
// Options can be provided to customize the queue's properties (e.g., thread safety).
func NewFromSlice[E any](cmp func(E, E) int, slice []E, ops ...Option[E]) (*Queue[E], error) {
	q, err := New(cmp, ops...)
	if err != nil {
		return nil, err
	}
	if len(slice) > 0 {
		for _, e := range slice {
			q.list.PushBack(e)
		}
	}
	return q, nil
}

// NewFromMapKeys creates and initializes a queue from the provided map keys.
// The "cmp" function is used to compare elements for equality.
// Options can be provided to customize the queue's properties (e.g., thread safety).
func NewFromMapKeys[K comparable, V any](cmp func(K, K) int, m map[K]V, ops ...Option[K]) (*Queue[K], error) {
	q, err := New(cmp, ops...)
	if err != nil {
		return nil, err
	}
	if len(m) > 0 {
		for k := range m {
			q.list.PushBack(k)
		}
	}
	return q, nil
}

// NewFromMapValues creates and initializes a queue from the provided map values.
// The "cmp" function is used to compare elements for equality.
// Options can be provided to customize the queue's properties (e.g., thread safety).
func NewFromMapValues[K comparable, V any](cmp func(V, V) int, m map[K]V, ops ...Option[V]) (*Queue[V], error) {
	q, err := New(cmp, ops...)
	if err != nil {
		return nil, err
	}
	if len(m) > 0 {
		for _, v := range m {
			q.list.PushBack(v)
		}
	}
	return q, nil
}

// Enqueue adds a element to the end of the queue.
func (q *Queue[E]) Enqueue(e E) {
	if q.safe {
		q.mu.Lock()
		defer q.mu.Unlock()
	}

	q.list.PushBack(e)
}

// Dequeue removes first element of the queue.
// Returns zero value of element and false if queue is empty.
func (q *Queue[E]) Dequeue() (E, bool) {
	if q.safe {
		q.mu.Lock()
		defer q.mu.Unlock()
	}

	var e E
	if q.list.Len() == 0 {
		return e, false
	}
	return q.list.PopFront(), true
}

// Peek returns first element of the queue without remove it.
// Returns zero value of element and false if queue is empty.
func (q *Queue[E]) Peek() (E, bool) {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}

	var e E
	if q.list.IsEmpty() {
		return e, false
	}
	return q.list.Head.Value, true
}

// IsEmpty reports whether the queue has no element.
func (q *Queue[E]) IsEmpty() bool {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}

	return q.list.IsEmpty()
}

// Len returns the number of elements currently in the queue.
func (q *Queue[E]) Len() int {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}

	return q.list.Len()
}

// Values returns all elements in the queue in FIFO(first-in, first-out) order.
func (q *Queue[E]) Values() []E {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}

	return q.list.Slice()
}

// Clear removes all elements from the queue.
func (q *Queue[E]) Clear() {
	if q.safe {
		q.mu.Lock()
		defer q.mu.Unlock()
	}

	q.list.Clear()
}

// Clone returns a deep copy of the queue.
func (q *Queue[E]) Clone() *Queue[E] {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}

	clone, _ := NewFromSlice(q.cmp, q.list.Slice(), q.options()...)
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

	items := make([]string, 0, q.list.Len())
	q.list.Range(func(v E) bool {
		items = append(items, fmt.Sprintf("%v", v))
		return true
	})
	return fmt.Sprintf("queue:{%s}", strings.Join(items, ", "))
}
