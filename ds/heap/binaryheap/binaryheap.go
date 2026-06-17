package binaryheap

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/hydroan/gst/ds/types"
)

// Heap is a generic binary heap implementation that support both max-heap and min-heap.
// By default, is operates as a min-heap where smallest element has the highest priority.
//
// To creates a max-heap where largest element has the highest priority,
// use "WithMaxHeap" option in the NewXXX function.
type Heap[E any] struct {
	data []E

	cmp func(E, E) int

	safe    bool
	mu      types.Locker
	maxHeap bool
}

// New creates and returns a min-heap with the given comparison function and options configurations.
// The comparison function cmp should return:
//   - zero value if a = b
//   - negative value if a < b
//   - positive value if a > b
//
// The provided options are provided to customize the heap property.
// For example:
//   - pass WithSafe to create a thread-safe heap.
//   - pass WithMaxHeap to create a max-heap  instead of the default min-heap.
func New[E any](cmp func(E, E) int, ops ...Option[E]) (*Heap[E], error) {
	if cmp == nil {
		return nil, types.ErrComparisonNil
	}
	h := &Heap[E]{data: make([]E, 0), cmp: cmp, mu: types.FakeLocker{}}
	for _, op := range ops {
		if op == nil {
			continue
		}
		if err := op(h); err != nil {
			return nil, err
		}
	}
	return h, nil
}

// NewOrdered is a shortcut for New(cmp.Compare, ops...)
// more detailed see "New"
func NewOrdered[E cmp.Ordered](ops ...Option[E]) (*Heap[E], error) {
	return New(cmp.Compare, ops...)
}

// NewFromSlice creates and returns a min-heap from the given slice.
// more detailed see "New"
func NewFromSlice[E any](slice []E, cmp func(E, E) int, ops ...Option[E]) (*Heap[E], error) {
	h, err := New(cmp, ops...)
	if err != nil {
		return nil, err
	}
	for _, v := range slice {
		h.Push(v)
	}
	return h, nil
}

// NewFromOrderedSlice is a shortcut for NewFromSlice(cmp.Ordered, ops...).
// more detailed see "New"
func NewFromOrderedSlice[E cmp.Ordered](slice []E, ops ...Option[E]) (*Heap[E], error) {
	h, err := NewOrdered(ops...)
	if err != nil {
		return nil, err
	}
	for _, v := range slice {
		h.Push(v)
	}
	return h, nil
}

// NewFromMapKeys creates and returns a min-heap from the given map keys.
// more detailed see "New"
func NewFromMapKeys[K comparable](m map[K]any, cmp func(K, K) int, ops ...Option[K]) (*Heap[K], error) {
	h, err := New(cmp, ops...)
	if err != nil {
		return nil, err
	}
	for k := range m {
		h.Push(k)
	}
	return h, nil
}

// NewFromMapValues creates and returns a min-heap from the given map values.
// more detailed see "New"
func NewFromMapValues[K comparable, V comparable](m map[K]V, cmp func(V, V) int, ops ...Option[V]) (*Heap[V], error) {
	h, err := New(cmp, ops...)
	if err != nil {
		return nil, err
	}
	for _, v := range m {
		h.Push(v)
	}
	return h, nil
}

// NewFromOrderedMapKeys is a shortcut for NewFromMapKeys(cmp.Ordered, ops...).
// more detailed see "New"
func NewFromOrderedMapKeys[K cmp.Ordered, V any](m map[K]V, ops ...Option[K]) (*Heap[K], error) {
	h, err := NewOrdered(ops...)
	if err != nil {
		return nil, err
	}
	for k := range m {
		h.Push(k)
	}
	return h, nil
}

// NewFromOrderedMapValues is a shortcut for NewFromMapValues(cmp.Ordered, ops...).
// more detailed see "New"
func NewFromOrderedMapValues[K comparable, V cmp.Ordered](m map[K]V, ops ...Option[V]) (*Heap[V], error) {
	h, err := NewOrdered(ops...)
	if err != nil {
		return nil, err
	}
	for _, v := range m {
		h.Push(v)
	}
	return h, nil
}

// Push inserts the given element into the heap.
func (h *Heap[E]) Push(v E) {
	if h.safe {
		h.mu.Lock()
		defer h.mu.Unlock()
	}

	h.push(v)
}

func (h *Heap[E]) push(e E) {
	h.data = append(h.data, e)
	if h.maxHeap {
		h.upMaxHeap(len(h.data) - 1)
	} else {
		h.upMinHeap(len(h.data) - 1)
	}
}

// Pop removes the root element from the heap and returns it.
func (h *Heap[E]) Pop() (E, bool) {
	if h.safe {
		h.mu.Lock()
		defer h.mu.Unlock()
	}

	return h.pop()
}

func (h *Heap[E]) pop() (E, bool) {
	if len(h.data) == 0 {
		var e E
		return e, false
	}
	d := h.data[0] // the deleted element
	if len(h.data) > 1 {
		h.data[0] = h.data[len(h.data)-1] // remove the root(first) element
		h.data = h.data[:len(h.data)-1]   // remove the left(last) element
		if h.maxHeap {
			h.downMaxHeap(0) // sink down the element
		} else {
			h.downMinHeap(0)
		}
	} else {
		h.data = h.data[:0] // remove the last element
	}
	return d, true
}

// Peek returns the root element from the heap without removing it.
func (h *Heap[E]) Peek() (E, bool) {
	if h.safe {
		h.mu.RLock()
		defer h.mu.RUnlock()
	}

	if len(h.data) == 0 {
		var e E
		return e, false
	}
	return h.data[0], true
}

// IsEmpty reports whether the heap is empty.
func (h *Heap[E]) IsEmpty() bool {
	if h.safe {
		h.mu.RLock()
		defer h.mu.RUnlock()
	}

	return len(h.data) == 0
}

// Size returns the number of elements in the heap.
func (h *Heap[E]) Size() int {
	if h.safe {
		h.mu.RLock()
		defer h.mu.RUnlock()
	}

	return len(h.data)
}

// Clear removes all elements from the heap.
func (h *Heap[E]) Clear() {
	if h.safe {
		h.mu.Lock()
		defer h.mu.Unlock()
	}
	h.data = make([]E, 0)
}

// Values returns a slice of all elements in the heap.
func (h *Heap[E]) Values() []E {
	if h.safe {
		h.mu.RLock()
		defer h.mu.RUnlock()
	}

	return h.values()
}

func (h *Heap[E]) values() []E {
	copied := h.clone()
	slice := make([]E, 0, len(h.data))
	for len(copied.data) > 0 {
		e, _ := copied.pop()
		slice = append(slice, e)
	}
	return slice
}

// Clone returns a copy of the heap.
func (h *Heap[E]) Clone() *Heap[E] {
	if h.safe {
		h.mu.RLock()
		defer h.mu.RUnlock()
	}

	return h.clone()
}

func (h *Heap[E]) clone() *Heap[E] {
	hh, _ := New(h.cmp, h.options()...)
	for _, e := range h.data {
		hh.push(e)
	}
	return hh
}

func (h *Heap[E]) options() []Option[E] {
	ops := make([]Option[E], 0)
	if h.safe {
		ops = append(ops, WithSafe[E]())
	}
	if h.maxHeap {
		ops = append(ops, WithMaxHeap[E]())
	}
	return ops
}

// Range call then given function on each element in the heap.
// If the function returns false, the iteration stops.
func (h *Heap[E]) Range(fn func(e E) bool) {
	if fn == nil {
		return
	}
	if h.safe {
		h.mu.RLock()
		defer h.mu.RUnlock()
	}

	for _, e := range h.data {
		if !fn(e) {
			return
		}
	}
}

// // String returns a string repesentation of the heap.
// func (h *Heap[E]) String() string {
// }

// String returns a string representation of the heap in a tree-like format.
func (h *Heap[E]) String() string {
	if h.safe {
		h.mu.RLock()
		defer h.mu.RUnlock()
	}

	if len(h.data) == 0 {
		return "BinaryHeap (empty)"
	}

	var sb strings.Builder
	sb.WriteString("BinaryHeap\n")
	output(0, "", true, &sb, h.data)
	return sb.String()
}

func output[E any](idx int, prefix string, isTail bool, sb *strings.Builder, data []E) {
	// Process right child first
	right := 2*idx + 2
	if right < len(data) {
		newPrefix := prefix
		if isTail {
			newPrefix += "│   "
		} else {
			newPrefix += "    "
		}
		output(right, newPrefix, false, sb, data)
	}

	// Process current node
	sb.WriteString(prefix)
	if isTail {
		sb.WriteString("╰── ")
	} else {
		sb.WriteString("╭── ")
	}
	fmt.Fprintf(sb, "%v\n", data[idx])

	// Process left child
	left := 2*idx + 1
	if left < len(data) {
		newPrefix := prefix
		if isTail {
			newPrefix += "    "
		} else {
			newPrefix += "│   "
		}
		output(left, newPrefix, true, sb, data)
	}
}

// upMinHeap will bubble up the element at index "i" until the min-heap property is restored.
func (h *Heap[E]) upMinHeap(i int) {
	for {
		p := (i - 1) / 2 // parent index
		if i <= 0 || h.cmp(h.data[i], h.data[p]) >= 0 {
			break
		}
		h.data[p], h.data[i] = h.data[i], h.data[p]
		i = p
	}
}

// upMaxHeap will bubble up the element at index "i" until the max-heap property is restored.
func (h *Heap[E]) upMaxHeap(i int) {
	for {
		p := (i - 1) / 2 // parent index
		if i <= 0 || h.cmp(h.data[i], h.data[p]) <= 0 {
			break
		}
		h.data[p], h.data[i] = h.data[i], h.data[p]
		i = p
	}
}

// downMinHeap will sink down the given element at index "i" until the min-heap property is restored.
func (h *Heap[E]) downMinHeap(i int) {
	for {
		l, r := 2*i+1, 2*i+2
		if l >= len(h.data) || l < 0 { // over range or overflow
			break
		}

		// find the minimum element
		m := l // the minimum element index
		if r < len(h.data) && h.cmp(h.data[r], h.data[l]) < 0 {
			m = r
		}
		// if the current element is less than or equal to the minimum element, break
		if h.cmp(h.data[i], h.data[m]) <= 0 {
			break
		}
		// swap the current element with the minimum element
		h.data[i], h.data[m] = h.data[m], h.data[i]
		i = m
	}
}

// downMaxHeap will sink down the given element at index "i" until the max-heap property is restored.
func (h *Heap[E]) downMaxHeap(i int) {
	for {
		l, r := 2*i+1, 2*i+2
		if l >= len(h.data) || l < 0 { // over range or overflow
			break
		}

		// find the maximum element
		m := l // the maximum element index
		if r < len(h.data) && h.cmp(h.data[r], h.data[l]) > 0 {
			m = r
		}
		// if the current element is greater than or equal to the maximum element, break
		if h.cmp(h.data[i], h.data[m]) >= 0 {
			break
		}
		// swap the current element with the maximum element
		h.data[i], h.data[m] = h.data[m], h.data[i]
		i = m
	}
}
