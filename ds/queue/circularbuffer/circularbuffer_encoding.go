package circularbuffer

import (
	"encoding/json"
)

// MarshalJSON will marshal the buffer into a JSON array, the oldest element
// first.
func (cb *Buffer[E]) MarshalJSON() ([]byte, error) {
	if cb.safe {
		cb.mu.RLock()
		defer cb.mu.RUnlock()
	}

	if cb.size == 0 {
		return []byte("[]"), nil
	}
	elements := make([]E, 0, cb.size)
	idx := cb.head
	size := len(cb.buf)
	for range cb.size {
		elements = append(elements, cb.buf[idx])
		idx = (idx + 1) % size
	}
	return json.Marshal(elements)
}

// UnmarshalJSON replaces the elements of the buffer with those of a JSON
// array, enqueued in order. The buffer keeps its size, so an array longer
// than it is handled as Enqueue handles a full buffer.
func (cb *Buffer[E]) UnmarshalJSON(data []byte) error {
	if cb.safe {
		cb.mu.Lock()
		defer cb.mu.Unlock()
	}

	buf := make([]E, 0)
	if err := json.Unmarshal(data, &buf); err != nil {
		return err
	}
	cb.buf = make([]E, cb.maxSize)
	cb.head = 0
	cb.tail = 0
	cb.size = 0
	for _, e := range buf {
		cb.enqueue(e)
	}

	return nil
}
