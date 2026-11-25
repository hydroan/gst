package splaytree

import (
	"github.com/hydroan/gst/ds/container"
)

// Option is a functional option type for configuring a splay tree.
type Option[K comparable, V any] func(*Tree[K, V]) error

// WithSafe creates an Option that makes the splay tree safe for concurrent use.
func WithSafe[K comparable, V any]() Option[K, V] {
	return func(t *Tree[K, V]) error {
		t.safe = true
		return nil
	}
}

// WithNodeFormatter creates an Option that sets how tree.String() prints a
// node, which by default is its key alone.
// Example usage:
//
//	tree, _ := splaytree.NewOrdered(splaytree.WithNodeFormatter(func(k string, v int) string {
//		return fmt.Sprintf("%s:%d", k, v)
//	}))
func WithNodeFormatter[K comparable, V any](fn func(K, V) string) Option[K, V] {
	return func(t *Tree[K, V]) error {
		if fn == nil {
			return container.ErrFuncNil
		}
		t.nodeFormatter = fn
		return nil
	}
}
