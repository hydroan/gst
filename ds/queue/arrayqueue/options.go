package arrayqueue

import "sync"

type Option[E any] func(*Queue[E]) error

// WithSafe creates a Option that make the array-backed queue safe for concurrent use.
func WithSafe[E any]() Option[E] {
	return func(l *Queue[E]) error {
		l.safe = true
		l.mu = &sync.RWMutex{}
		return nil
	}
}
