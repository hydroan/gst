package linkedlist

import "sync"

type Option[V any] func(*List[V]) error

// WithSafe creates an Option that makes the doubly-linked list safe for concurrent use.
func WithSafe[V any]() Option[V] {
	return func(m *List[V]) error {
		m.mu = new(sync.RWMutex)
		m.safe = true
		return nil
	}
}
