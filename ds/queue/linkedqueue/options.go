package linkedqueue

// Option is a functional option type for configuring a Queue.
type Option[E any] func(*Queue[E]) error

// WithSafe creates an Option that makes the linkedlist-backed queue safe for
// concurrent use.
func WithSafe[E any]() Option[E] {
	return func(l *Queue[E]) error {
		l.safe = true
		return nil
	}
}
