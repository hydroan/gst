package splaytree

import (
	"strings"
	"sync"
)

// Option is a functional option type for configuring a splay tree.
type Option[K comparable, V any] func(*Tree[K, V]) error

// WithSafe creates a option that make the splay tree safe for concurrent use.
func WithSafe[K comparable, V any]() Option[K, V] {
	return func(t *Tree[K, V]) error {
		t.safe = true
		t.mu = &sync.RWMutex{}
		return nil
	}
}

// WithNodeFormat creates a option that sets the node format when call tree.String().
// Default node format is fmt.Sprintf("%v", n.Key).
// The format must contains two placeholders, the first is the key and the second is the value.
// For example: "%d:%s"
func WithNodeFormat[K comparable, V any](format string) Option[K, V] {
	return func(t *Tree[K, V]) error {
		format = strings.TrimFunc(format, func(r rune) bool {
			return r == '\n'
		})
		if len(format) > 0 {
			t.nodeFormat = format + "\n"
		}
		return nil
	}
}
