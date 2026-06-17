package skiplist

import "encoding/json"

type pair[K comparable, V any] struct {
	Key   K `json:"key"`
	Value V `json:"value"`
}

// MarshalJSON implements the json.Marshaler interface.
func (sl *SkipList[K, V]) MarshalJSON() ([]byte, error) {
	if sl.safe {
		sl.mu.RLock()
		defer sl.mu.RUnlock()
	}

	if sl.size == 0 || sl.head == nil {
		return []byte("[]"), nil
	}
	pairs := make([]pair[K, V], 0, sl.size)
	curr := sl.head.next[0]
	for curr != nil {
		pairs = append(pairs, pair[K, V]{Key: curr.Key, Value: curr.Value})
		curr = curr.next[0]
	}
	return json.Marshal(pairs)
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (sl *SkipList[K, V]) UnmarshalJSON(data []byte) error {
	if sl.safe {
		sl.mu.Lock()
		defer sl.mu.Unlock()
	}

	sl.head = &Node[K, V]{next: make([]*Node[K, V], sl.maxLevel)}
	sl.level = 1
	sl.size = 0

	var pairs []pair[K, V]
	if err := json.Unmarshal(data, &pairs); err != nil {
		return err
	}
	for _, p := range pairs {
		sl.put(p.Key, p.Value)
	}
	return nil
}
