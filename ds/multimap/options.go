package multimap

import "sync"

type Option[K comparable, V any] func(*MultiMap[K, V]) error

// WithSafe creates an Option that makes the MultiMap safe for concurrent use.
func WithSafe[K comparable, V any]() Option[K, V] {
	return func(m *MultiMap[K, V]) error {
		m.mu = new(sync.RWMutex)
		m.safe = true
		return nil
	}
}
