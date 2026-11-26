package rbtree

import (
	"github.com/hydroan/gst/ds/container"
)

// Option is a functional option type for configuring a red-black tree.
type Option[K comparable, V any] func(t *Tree[K, V]) error

// WithSafe creates an Option that makes the red-black tree safe for concurrent
// use.
func WithSafe[K comparable, V any]() Option[K, V] {
	return func(t *Tree[K, V]) error {
		t.safe = true
		return nil
	}
}

// WithNodeFormatter creates an Option that sets the node formatter when
// tree.String() is called. Example usage:
//
//	tree, _ := rbtree.NewOrdered(rbtree.WithNodeFormatter(func(k string, v int) string {
//		return fmt.Sprintf("%s:%d ", k, v)
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

// WithColorfulString creates an Option that makes tree.String() print the
// red-black tree in color.
func WithColorfulString[K comparable, V any]() Option[K, V] {
	return func(t *Tree[K, V]) error {
		t.color = true
		return nil
	}
}
