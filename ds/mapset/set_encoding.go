package mapset

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MarshalJSON will marshal the set into a JSON-based representation.
func (s *Set[E]) MarshalJSON() ([]byte, error) {
	if s.safe {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}

	items := make([]string, 0, len(s.set))
	if s.sorted {
		elements := s.sortedSlice(s.cmp)
		for _, e := range elements {
			b, err := json.Marshal(e)
			if err != nil {
				return nil, err
			}
			items = append(items, string(b))
		}
	} else {
		for e := range s.set {
			b, err := json.Marshal(e)
			if err != nil {
				return nil, err
			}
			items = append(items, string(b))
		}
	}

	return fmt.Appendf(nil, "[%s]", strings.Join(items, ",")), nil
}

// UnmarshalJSON will unmarshal a JSON-based representation byte slice into the set.
func (s *Set[E]) UnmarshalJSON(data []byte) error {
	if s.safe {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

	var items []E
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	s.set = make(map[E]struct{}, len(items))
	for _, e := range items {
		s.set[e] = struct{}{}
	}
	return nil
}
