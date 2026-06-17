package linkedstack

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// MarshalJSON will marshal the stack into a JSON-based representation.
func (s *Stack[E]) MarshalJSON() ([]byte, error) {
	if s.safe {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}

	el := s.list.Slice()
	slices.Reverse(el)
	items := make([]string, 0, s.list.Len())
	for _, e := range el {
		b, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		items = append(items, string(b))
	}
	return fmt.Appendf(nil, "[%s]", strings.Join(items, ",")), nil
}

// UnmarshalJSON will unmarshal a JSON-based representation byte slice into the stack.
func (s *Stack[E]) UnmarshalJSON(data []byte) (err error) {
	if s.safe {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

	el := make([]E, 0)
	if err = json.Unmarshal(data, &el); err != nil {
		return err
	}
	s.list.Clear()
	for _, e := range el {
		s.list.PushBack(e)
	}
	return nil
}
