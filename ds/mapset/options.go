package mapset

import (
	"sync"

	"github.com/hydroan/gst/ds/types"
)

// Option is a functional option type for configuring a Set.
type Option[T comparable] func(*Set[T]) error

// WithSafe creates a option that make the Set safe for concurrent use.
func WithSafe[E comparable]() Option[E] {
	return func(s *Set[E]) error {
		s.mu = new(sync.RWMutex)
		s.safe = true
		return nil
	}
}

// WithSorted creates a option that ensure the set to always makeup a sorted
// order elements based on the provided compator function.
//
// This affects the behavior of the following methods:
// - "Slice": Returns a sorted slice of the elements in the set.
// - "String": Returns a string representation of the set with sorted elements.
// - "MarshalJSON": sorts the elements before returning the JSON representation.
// - "Range": sorts the elements before calling the provided function.
// - "Iter": sorts the elements before returning a channel of elements.
func WithSorted[E comparable](cmp func(E, E) int) Option[E] {
	return func(s *Set[E]) error {
		if cmp == nil {
			return types.ErrComparisonNil
		}
		s.sorted = true
		s.cmp = cmp
		return nil
	}
}
