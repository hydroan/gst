package linkedlist

import (
	"sync"

	"github.com/hydroan/gst/ds/types"
)

type Option[V any] func(*List[V]) error

// WithSafe creates a Option that make the doublely-linked list safe for concurrent use.
func WithSafe[V any]() Option[V] {
	return func(m *List[V]) error {
		m.mu = new(sync.RWMutex)
		m.safe = true
		return nil
	}
}

// WithSorted creates a option that ensure the doublely-linked to always makeup a sorted
// order elements based on the provided compator function.
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
