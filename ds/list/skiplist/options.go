package skiplist

import (
	"fmt"
	"sync"

	"github.com/hydroan/gst/ds/types"
)

// Option is a function type that can be used to configure skiplist.
type Option[K comparable, V any] func(*SkipList[K, V]) error

// WithSafe creates a Option to configures the skiplist to be thread-safe.
func WithSafe[K comparable, V any]() Option[K, V] {
	return func(l *SkipList[K, V]) error {
		l.safe = true
		l.mu = &sync.RWMutex{}
		return nil
	}
}

// WithMaxLevel creates an Option to set the maximum level for the skiplist.
// The max level must be greater than 0.
func WithMaxLevel[K comparable, V any](maxLevel int) Option[K, V] {
	return func(sl *SkipList[K, V]) error {
		if maxLevel <= 0 {
			return fmt.Errorf("max level must be greater than 0, got %d", maxLevel)
		}
		sl.head.next = make([]*Node[K, V], maxLevel)
		sl.maxLevel = maxLevel
		return nil
	}
}

// WithProbability creates an Option to set the probability factor for the skiplist.
// The probability must be in the range (0,1).
func WithProbability[K comparable, V any](p float64) Option[K, V] {
	return func(sl *SkipList[K, V]) error {
		if p < 0.0 || p > 1.0 {
			return fmt.Errorf("probability must be in range (0,1), got %v", p)
		}
		sl.p = p
		return nil
	}
}

// WithNodeFormatter creates an Option to set the node formatter for the skiplist.
// Affects method: String().
func WithNodeFormatter[K comparable, V any](fn func(k K, v V) string) Option[K, V] {
	return func(sl *SkipList[K, V]) error {
		if fn == nil {
			return types.ErrFuncNil
		}
		sl.nodeFormatter = fn
		return nil
	}
}
