package priorityqueue

import "sync"

// Option is a function type that can be used to configures properties of a priority queue..
type Option[E any] func(q *Queue[E]) error

// WithSafe creates a Option to configures a priority queue to be thread-safe.
func WithSafe[E any]() Option[E] {
	return func(q *Queue[E]) error {
		q.safe = true
		q.mu = &sync.RWMutex{}
		return nil
	}
}

// WithMaxPriority returns an Option that configures the priority queue to use max-heap ordering,
// where larger values have higher priority. By default, the queue uses min-heap ordering
// (smaller values have higher priority).
func WithMaxPriority[E any]() Option[E] {
	return func(q *Queue[E]) error {
		q.maxPriority = true
		return nil
	}
}
