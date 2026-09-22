package linkedlist

import (
	"sync"

	"github.com/hydroan/gst/ds/types"
)

type Option[V any] func(*List[V]) error

// WithSafe creates an Option that makes the doubly-linked list safe for concurrent use.
func WithSafe[V any]() Option[V] {
	return func(m *List[V]) error {
		m.mu = new(sync.RWMutex)
		m.safe = true
		return nil
	}
}

// WithSorted creates an Option that records cmp as the order of the
// doubly-linked list. No list operation reads it yet: elements stay where the
// operations put them, and MergeSorted takes a comparison function of its own.
func WithSorted[V any](cmp func(V, V) int) Option[V] {
	return func(m *List[V]) error {
		m.sorted = true
		if cmp == nil {
			return types.ErrComparisonNil
		}
		m.cmp = cmp
		return nil
	}
}
