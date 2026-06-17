package rbtree

import (
	"sync"

	"github.com/hydroan/gst/ds/types"
)

// Option is a functional option type for configuring a red-black tree.
type Option[K comparable, V any] func(t *Tree[K, V]) error

// WithSafe creates a option that make the red-black tree safe for concurrent use.
func WithSafe[K comparable, V any]() Option[K, V] {
	return func(t *Tree[K, V]) error {
		t.safe = true
		t.mu = &sync.RWMutex{}
		return nil
	}
}

// WithNodeFormatter creates a option that sets the node formatter when call tree.String().
// Example usage:
//
//	tree.WithNodeFormatter(func(k string, v int) string {
//		return fmt.Sprintf("%s:%d ", k, v)
//	})
func WithNodeFormatter[K comparable, V any](fn func(K, V) string) Option[K, V] {
	return func(t *Tree[K, V]) error {
		if fn == nil {
			return types.ErrFuncNil
		}
		t.nodeFormatter = fn
		return nil
	}
}

// WithColorfulString creates a option that make the red-black tree colorful output.
func WithColorfulString[K comparable, V any]() Option[K, V] {
	return func(t *Tree[K, V]) error {
		t.color = true
		return nil
	}
}
