package linkedstack

// Option is a functional option type for configuring a Stack.
type Option[E any] func(*Stack[E]) error

// WithSafe creates an Option that makes the linkedlist-backed stack safe for concurrent use.
func WithSafe[E any]() Option[E] {
	return func(s *Stack[E]) error {
		s.safe = true
		return nil
	}
}
