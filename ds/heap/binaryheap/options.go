package binaryheap

// Option is a function type that configures a property of a binary heap.
type Option[T any] func(*Heap[T]) error

// WithSafe returns an Option to configure a binary heap to be thread-safe.
func WithSafe[T any]() Option[T] {
	return func(h *Heap[T]) error {
		h.safe = true
		return nil
	}
}

// WithMaxHeap returns an Option to configure a binary heap to be a max-heap.
func WithMaxHeap[T any]() Option[T] {
	return func(h *Heap[T]) error {
		h.maxHeap = true
		return nil
	}
}
