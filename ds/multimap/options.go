package multimap

import "sync"

type Option[K comparable, V any] func(*MultiMap[K, V]) error

// WithSafe creates a Option that make the MultiMap safe for concurrent use.
func WithSafe[K comparable, V any]() Option[K, V] {
	return func(m *MultiMap[K, V]) error {
		m.mu = new(sync.RWMutex)
		return nil
	}
}
