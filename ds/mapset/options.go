package mapset

import (
	"github.com/hydroan/gst/ds/container"
)

// Option is a functional option type for configuring a Set.
type Option[T comparable] func(*Set[T]) error

// WithSafe creates an Option that makes the Set safe for concurrent use.
func WithSafe[E comparable]() Option[E] {
	return func(s *Set[E]) error {
		s.safe = true
		return nil
	}
}

// WithSorted creates an Option that orders the elements of the set by the
// provided comparison function wherever the set hands them out.
//
// This affects the behavior of the following methods:
//   - "Slice": Returns a sorted slice of the elements in the set.
//   - "String": Returns a string representation of the set with sorted
//     elements.
//   - "MarshalJSON": sorts the elements before returning the JSON
//     representation.
//   - "Range": sorts the elements before calling the provided function.
//   - "Iter": sorts the elements before returning a channel of elements.
//   - "Seq": sorts the elements before yielding them to the iterator.
func WithSorted[E comparable](cmp func(E, E) int) Option[E] {
	return func(s *Set[E]) error {
		if cmp == nil {
			return container.ErrComparisonNil
		}
		s.sorted = true
		s.cmp = cmp
		return nil
	}
}
