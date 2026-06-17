package trie

import (
	"sync"

	"github.com/hydroan/gst/ds/types"
)

// Option is a function option type for configuring a trie.
type Option[K comparable, V any] func(*Trie[K, V]) error

// WithSafe returns a option that makes the trie safe for concurrent use.
func WithSafe[K comparable, V any]() Option[K, V] {
	return func(t *Trie[K, V]) error {
		t.safe = true
		t.mu = &sync.RWMutex{}
		return nil
	}
}

// WithNodeFormatter creates a option that sets the node formatter when call trie.String().
// Example usage:
//
//	trie.WithNodeFormatter(func(v int, c count) string {
//	    return fmt.Sprintf("value: %v, count: %d", v, c)
//	})
func WithNodeFormatter[K comparable, V any](fn func(V, int, bool) string) Option[K, V] {
	return func(t *Trie[K, V]) error {
		if fn == nil {
			return types.ErrFuncNil
		}
		t.nodeFormatter = fn
		return nil
	}
}

// WithKeyFormatter creates a option that sets the key formatter when call trie.String().
// Example usage:
//
//	trie.WithKeyFormatter(func(k string, n *Node[string, int]) string {
//	    return fmt.Sprintf("[%s]", k)
//	})
func WithKeyFormatter[K comparable, V any](fn func(K, V, int, bool) string) Option[K, V] {
	return func(t *Trie[K, V]) error {
		if fn == nil {
			return types.ErrFuncNil
		}
		t.keyFormatter = fn
		return nil
	}
}
