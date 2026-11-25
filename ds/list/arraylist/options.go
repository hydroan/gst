package arraylist

// Option is a functional option type for configuring a List.
type Option[E any] func(*List[E]) error

// WithSafe creates an Option that makes the array-backed list safe for concurrent use.
func WithSafe[E any]() Option[E] {
	return func(l *List[E]) error {
		l.safe = true
		return nil
	}
}
