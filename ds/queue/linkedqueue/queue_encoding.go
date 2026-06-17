package linkedqueue

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

	items := make([]string, 0, q.list.Len())
	var b []byte
	var err error
	q.list.Range(func(e E) bool {
		if b, err = json.Marshal(e); err == nil {
			items = append(items, string(b))
			return true
		}
		return false
	})
	if err != nil {
		return nil, err
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
	q.list.Clear()
	for _, e := range el {
		q.list.PushBack(e)
	}
	return nil
}
