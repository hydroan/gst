package circularbuffer

import (
	"sync"
)

// Option is a function that can be used to configure a CircularBuffer.
type Option[T any] func(*CircularBuffer[T]) error

// WithSafe returns an Option that configures the CircularBuffer to be thread-safe.
func WithSafe[T any]() Option[T] {
	return func(cb *CircularBuffer[T]) error {
		cb.safe = true
		cb.mu = &sync.RWMutex{}
		return nil
	}
}

// WithDrop returns an Option that configures the CircularBuffer to drop elements when full.
func WithDrop[T any]() Option[T] {
	return func(cb *CircularBuffer[T]) error {
		cb.drop = true
		return nil
	}
}
