package skiplist

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/ds/container"
)

// Option is a function type that can be used to configure skiplist.
type Option[K comparable, V any] func(*List[K, V]) error

// WithSafe creates an Option to configure the skiplist to be thread-safe.
func WithSafe[K comparable, V any]() Option[K, V] {
	return func(l *List[K, V]) error {
		l.safe = true
		return nil
	}
}

// WithMaxLevel creates an Option to set the maximum level for the skiplist.
// The max level must be greater than 0.
func WithMaxLevel[K comparable, V any](maxLevel int) Option[K, V] {
	return func(sl *List[K, V]) error {
		if maxLevel <= 0 {
			return errors.Newf("max level must be greater than 0, got %d", maxLevel)
		}
		sl.head.next = make([]*Node[K, V], maxLevel)
		sl.maxLevel = maxLevel
		return nil
	}
}

// WithProbability creates an Option to set the probability factor for the
// skiplist. The probability must be in the range (0,1).
func WithProbability[K comparable, V any](p float64) Option[K, V] {
	return func(sl *List[K, V]) error {
		if p < 0.0 || p > 1.0 {
			return errors.Newf("probability must be in range (0,1), got %v", p)
		}
		sl.p = p
		return nil
	}
}

// WithNodeFormatter creates an Option to set the node formatter for the
// skiplist. Affects method: String().
func WithNodeFormatter[K comparable, V any](fn func(k K, v V) string) Option[K, V] {
	return func(sl *List[K, V]) error {
		if fn == nil {
			return container.ErrFuncNil
		}
		sl.nodeFormatter = fn
		return nil
	}
}
