// Package multimap is a generic map holding any number of values under each
// key, the values told apart by a comparison function. WithSafe makes a map
// safe for concurrent use.
package multimap

import (
	"slices"
	"sync"

	"github.com/hydroan/gst/ds/container"
)

// MultiMap represents a map that can store multiple values for each key. Call
// New with the `WithSafe` option to make the MultiMap safe for concurrent use.
type MultiMap[K comparable, V any] struct {
	data map[K][]V
	cmp  func(V, V) int
	mu   sync.RWMutex
	safe bool
}

// New creates an empty MultiMap. cmp compares values: two values are equal
// when it returns 0. A nil cmp fails with container.ErrComparisonNil.
func New[K comparable, V any](cmp func(V, V) int, ops ...Option[K, V]) (*MultiMap[K, V], error) {
	if cmp == nil {
		return nil, container.ErrComparisonNil
	}
	m := &MultiMap[K, V]{
		data: make(map[K][]V),
		cmp:  cmp,
	}
	for _, op := range ops {
		if op == nil {
			continue
		}
		if err := op(m); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// NewFromMap creates a MultiMap from an existing map, performing a deep copy
// of the input map's values. A key with no values is left out, as Set leaves
// it out.
func NewFromMap[K comparable, V any](m map[K][]V, cmp func(V, V) int, ops ...Option[K, V]) (*MultiMap[K, V], error) {
	mm, err := New[K, V](cmp, ops...)
	if err != nil {
		return nil, err
	}
	for k, values := range m {
		mm.Set(k, values...)
	}
	return mm, nil
}

// Get returns all values associated with the key and whether the key exists.
// The returned slice is a copy of the internal slice to prevent modifications.
func (m *MultiMap[K, V]) Get(key K) ([]V, bool) {
	if m.safe {
		m.mu.RLock()
		defer m.mu.RUnlock()
	}

	values, exists := m.data[key]
	if !exists {
		return nil, false
	}
	return slices.Clone(values), true
}

// GetOne returns the first value associated with the key and whether it exists.
// If the key doesn't exist, it returns the zero value and false.
func (m *MultiMap[K, V]) GetOne(key K) (V, bool) {
	if m.safe {
		m.mu.RLock()
		defer m.mu.RUnlock()
	}

	values, exists := m.data[key]
	if !exists {
		var zero V
		return zero, false
	}
	return values[0], true
}

// Add appends val to the values associated with the key.
func (m *MultiMap[K, V]) Add(key K, val V) {
	if m.safe {
		m.mu.Lock()
		defer m.mu.Unlock()
	}

	m.data[key] = append(m.data[key], val)
}

// Set replaces the values associated with the key with a copy of values.
// Setting no values removes the key, the way removing its last value does.
func (m *MultiMap[K, V]) Set(key K, values ...V) {
	if m.safe {
		m.mu.Lock()
		defer m.mu.Unlock()
	}

	if len(values) == 0 {
		delete(m.data, key)
		return
	}
	m.data[key] = slices.Clone(values)
}

// Delete removes the key and all its associated values from the map.
func (m *MultiMap[K, V]) Delete(key K) {
	if m.safe {
		m.mu.Lock()
		defer m.mu.Unlock()
	}

	delete(m.data, key)
}

// DeleteValue removes all occurrences of val from the values associated with
// the key. If all values are removed, the key is also removed. Returns the
// number of values deleted.
func (m *MultiMap[K, V]) DeleteValue(key K, val V) int {
	if m.safe {
		m.mu.Lock()
		defer m.mu.Unlock()
	}

	values, exists := m.data[key]
	if !exists {
		return 0
	}

	newValues := make([]V, 0, len(values))
	count := 0
	for _, v := range values {
		if m.cmp(v, val) != 0 {
			newValues = append(newValues, v)
		} else {
			count++
		}
	}
	if len(newValues) == 0 {
		// If no values left, remove the key entirely
		delete(m.data, key)
	} else if count > 0 {
		m.data[key] = newValues
	}

	return count
}

// Len returns the total number of keys in the MultiMap.
func (m *MultiMap[K, V]) Len() int {
	if m.safe {
		m.mu.RLock()
		defer m.mu.RUnlock()
	}

	return len(m.data)
}

// Size returns the total number of values across all keys in the MultiMap.
func (m *MultiMap[K, V]) Size() int {
	if m.safe {
		m.mu.RLock()
		defer m.mu.RUnlock()
	}

	size := 0
	for _, values := range m.data {
		size += len(values)
	}
	return size
}

// IsEmpty reports whether the MultiMap has no key-values pairs.
func (m *MultiMap[K, V]) IsEmpty() bool {
	if m.safe {
		m.mu.RLock()
		defer m.mu.RUnlock()
	}

	return len(m.data) == 0
}

// Has reports whether the MultiMap contains the specified key.
func (m *MultiMap[K, V]) Has(key K) bool {
	if m.safe {
		m.mu.RLock()
		defer m.mu.RUnlock()
	}

	_, exists := m.data[key]
	return exists
}

// Contains reports whether the MultiMap contains the specified value for the
// given key.
func (m *MultiMap[K, V]) Contains(key K, val V) bool {
	if m.safe {
		m.mu.RLock()
		defer m.mu.RUnlock()
	}

	values, exists := m.data[key]
	if !exists {
		return false
	}

	for _, v := range values {
		if m.cmp(v, val) == 0 {
			return true
		}
	}
	return false
}

// Count returns the number of values associated with the given key.
// Returns 0 if the key does not exist.
func (m *MultiMap[K, V]) Count(key K) int {
	if m.safe {
		m.mu.RLock()
		defer m.mu.RUnlock()
	}

	return len(m.data[key])
}

// Keys returns a slice containing all keys in the MultiMap.
// The order of keys is not guaranteed.
func (m *MultiMap[K, V]) Keys() []K {
	if m.safe {
		m.mu.RLock()
		defer m.mu.RUnlock()
	}

	keys := make([]K, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys
}

// Values returns a slice containing all values across all keys in the MultiMap.
// The order of values is not guaranteed.
func (m *MultiMap[K, V]) Values() []V {
	if m.safe {
		m.mu.RLock()
		defer m.mu.RUnlock()
	}

	capacity := 0
	for _, values := range m.data {
		capacity += len(values)
	}
	values := make([]V, 0, capacity)
	for _, vs := range m.data {
		values = append(values, vs...)
	}
	return values
}

// Clear removes all key-values pairs from the MultiMap.
func (m *MultiMap[K, V]) Clear() {
	if m.safe {
		m.mu.Lock()
		defer m.mu.Unlock()
	}

	clear(m.data)
}

// Range iterates over the MultiMap and calls function fn for each key and its
// values. If fn returns false, iteration stops immediately. If fn is nil, Range
// returns without doing anything.
func (m *MultiMap[K, V]) Range(fn func(key K, values []V) bool) {
	if m.safe {
		m.mu.RLock()
		defer m.mu.RUnlock()
	}

	if fn == nil {
		return
	}
	for k, values := range m.data {
		if !fn(k, values) {
			return
		}
	}
}

// Clone returns a deep copy of the MultiMap, made with the same options and
// so with a lock of its own when the MultiMap is safe for concurrent use.
func (m *MultiMap[K, V]) Clone() *MultiMap[K, V] {
	if m.safe {
		m.mu.RLock()
		defer m.mu.RUnlock()
	}

	cloned, _ := New(m.cmp, m.options()...)
	for k, values := range m.data {
		cloned.data[k] = slices.Clone(values)
	}
	return cloned
}

func (m *MultiMap[K, V]) options() []Option[K, V] {
	ops := make([]Option[K, V], 0)
	if m.safe {
		ops = append(ops, WithSafe[K, V]())
	}
	return ops
}

// Map returns a deep copy of the internal map.
// The returned map can be safely modified without affecting the MultiMap.
func (m *MultiMap[K, V]) Map() map[K][]V {
	if m.safe {
		m.mu.RLock()
		defer m.mu.RUnlock()
	}

	result := make(map[K][]V, len(m.data))
	for k, values := range m.data {
		newValues := make([]V, len(values))
		copy(newValues, values)
		result[k] = newValues
	}
	return result
}
