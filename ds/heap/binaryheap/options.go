package binaryheap

import (
	"sync"
)

// Option is a function type that can be used to configures properties of a binary heap.
type Option[T any] func(*Heap[T]) error

// WithSafe return a Option to configure a binary heap to be thread-safe.
func WithSafe[T any]() Option[T] {
	return func(h *Heap[T]) error {
		h.safe = true
		h.mu = &sync.RWMutex{}
		return nil
	}
}

// WithMaxHeap return a Option to configure a binary heap to be a max-heap.
func WithMaxHeap[T any]() Option[T] {
	return func(h *Heap[T]) error {
		h.maxHeap = true
		return nil
	}
}
