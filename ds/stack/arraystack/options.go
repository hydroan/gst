package arraystack

import (
	"sync"
)

// Option is a functional option type for configuring a Stack.
type Option[E any] func(*Stack[E]) error

// WithSafe creates an Option that makes the array-backed stack safe for concurrent use.
func WithSafe[E any]() Option[E] {
	return func(s *Stack[E]) error {
		s.safe = true
		s.mu = &sync.RWMutex{}
		return nil
	}
}
