package binaryheap

import "encoding/json"

// MarshalJSON implements the json.Marshaler interface
func (h *Heap[E]) MarshalJSON() ([]byte, error) {
	if h.safe {
		h.mu.RLock()
		defer h.mu.RUnlock()
	}

	slice := h.values()
	return json.Marshal(slice)
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (h *Heap[E]) UnmarshalJSON(data []byte) error {
	if h.safe {
		h.mu.Lock()
		defer h.mu.Unlock()
	}

	var slice []E
	if err := json.Unmarshal(data, &slice); err != nil {
		return err
	}

	h.data = make([]E, 0, len(slice))
	for _, v := range slice {
		h.push(v)
	}
	return nil
}
