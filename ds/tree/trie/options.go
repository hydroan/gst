package trie

import (
	"sync"

	"github.com/hydroan/gst/ds/container"
)

// Option is a functional option type for configuring a trie.
type Option[K comparable, V any] func(*Trie[K, V]) error

// WithSafe returns an Option that makes the trie safe for concurrent use.
func WithSafe[K comparable, V any]() Option[K, V] {
	return func(t *Trie[K, V]) error {
		t.safe = true
		t.mu = &sync.RWMutex{}
		return nil
	}
}

// WithNodeFormatter creates an Option that sets the node formatter when trie.String() is called.
// Example usage:
//
//	trie.WithNodeFormatter(func(v int, c count) string {
//	    return fmt.Sprintf("value: %v, count: %d", v, c)
//	})
func WithNodeFormatter[K comparable, V any](fn func(V, int, bool) string) Option[K, V] {
	return func(t *Trie[K, V]) error {
		if fn == nil {
			return container.ErrFuncNil
		}
		t.nodeFormatter = fn
		return nil
	}
}

// WithKeyFormatter creates an Option that sets the key formatter when trie.String() is called.
// Example usage:
//
//	trie.WithKeyFormatter(func(k string, n *Node[string, int]) string {
//	    return fmt.Sprintf("[%s]", k)
//	})
func WithKeyFormatter[K comparable, V any](fn func(K, V, int, bool) string) Option[K, V] {
	return func(t *Trie[K, V]) error {
		if fn == nil {
			return container.ErrFuncNil
		}
		t.keyFormatter = fn
		return nil
	}
}
