package arraylist

import "sync"

type Option[E any] func(*List[E]) error

// WithSafe creates a Option that make the array-backed list safe for concurrent use.
func WithSafe[E any]() Option[E] {
	return func(l *List[E]) error {
		l.mu = new(sync.RWMutex)
		l.safe = true
		return nil
	}
}
