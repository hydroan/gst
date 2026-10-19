package arrayqueue

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MarshalJSON will marshal the queue into a JSON-based representation.
func (q *Queue[E]) MarshalJSON() ([]byte, error) {
	if q.safe {
		q.mu.RLock()
		defer q.mu.RUnlock()
	}

	items := make([]string, 0, len(q.elements))
	for _, e := range q.elements {
		b, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		items = append(items, string(b))
	}
	return fmt.Appendf(nil, "[%s]", strings.Join(items, ",")), nil
}

// UnmarshalJSON will unmarshal a JSON-based representation byte slice into the queue.
func (q *Queue[E]) UnmarshalJSON(data []byte) error {
	if q.safe {
		q.mu.Lock()
		defer q.mu.Unlock()
	}

	el := make([]E, 0)
	if err := json.Unmarshal(data, &el); err != nil {
		return err
	}
	q.elements = el
	return nil
}
