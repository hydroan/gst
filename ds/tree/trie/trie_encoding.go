package trie

import "encoding/json"

// MarshalJSON implements the json.Marshaler interface.
func (t *Trie[K, V]) MarshalJSON() ([]byte, error) {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}
	keysValues := t.keysValues()

	return json.Marshal(keysValues)
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (t *Trie[K, V]) UnmarshalJSON(data []byte) error {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	keysValues := make([]KeysValue[K, V], 0)
	if err := json.Unmarshal(data, &keysValues); err != nil {
		return err
	}
	t.root = nil
	t.size = 0
	for _, kv := range keysValues {
		t.put(kv.Keys, kv.Value)
	}

	return nil
}
