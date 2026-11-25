package circularbuffer

import (
	"sync"
)

// Option is a function that can be used to configure a Buffer.
type Option[T any] func(*Buffer[T]) error

// WithSafe returns an Option that configures the buffer to be thread-safe.
func WithSafe[T any]() Option[T] {
	return func(cb *Buffer[T]) error {
		cb.safe = true
		cb.mu = &sync.RWMutex{}
		return nil
	}
}

// WithDrop returns an Option that configures the buffer to drop elements when full.
func WithDrop[T any]() Option[T] {
	return func(cb *Buffer[T]) error {
		cb.drop = true
		return nil
	}
}
