package priorityqueue

// MarshalJSON implements the json.Marshaler interface.
func (q *Queue[E]) MarshalJSON() ([]byte, error) {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}

	return q.heap.MarshalJSON()
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (q *Queue[E]) UnmarshalJSON(data []byte) error {
	if q.safe {
		q.mu.Lock()
		defer q.mu.Unlock()
	}

	return q.heap.UnmarshalJSON(data)
}
