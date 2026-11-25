package linkedlist

// Option is a functional option type for configuring a List.
type Option[V any] func(*List[V]) error

// WithSafe creates an Option that makes the doubly-linked list safe for concurrent use.
func WithSafe[V any]() Option[V] {
	return func(l *List[V]) error {
		l.safe = true
		return nil
	}
}
