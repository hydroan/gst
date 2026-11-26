// Package arraylist provides a generic implementation of a resizable
// array-backed list.
package arraylist

import (
	"cmp"
	"slices"
	"sync"

	"github.com/hydroan/gst/ds/container"
)

const (
	growthFactor = float32(2.0)
	shrinkFactor = float32(0.25)

	minCap = 16
)

// List represents a resizable array-backed list. It is safe for concurrent
// use only when built with the WithSafe option.
type List[E any] struct {
	elements []E
	cmp      func(E, E) int

	// safe is set by WithSafe. Every method reads it before it takes mu, so
	// a list built without WithSafe does not pay for a lock it has no use for.
	safe bool
	mu   sync.RWMutex
}

// New creates and returns a new array-backed list. The provided cmp orders the
// elements: Sort sorts by it, and Contains, IndexOf and Remove treat two
// elements as equal when it returns 0. A nil cmp fails with
// container.ErrComparisonNil. Optional options can be passed to modify the
// list's behavior, such as enabling concurrent safety.
func New[E any](cmp func(E, E) int, ops ...Option[E]) (*List[E], error) {
	if cmp == nil {
		return nil, container.ErrComparisonNil
	}
	l := &List[E]{
		elements: make([]E, 0, minCap),
		cmp:      cmp,
	}
	for _, op := range ops {
		if op == nil {
			continue
		}
		if err := op(l); err != nil {
			return nil, err
		}
	}
	return l, nil
}

// NewOrdered creates and returns a new array-backed list for ordered elements.
// It uses cmp.Compare[E] as the default comparison function for elements.
// Optional options can be passed to modify the list's behavior, such as
// enabling concurrent safety.
func NewOrdered[E cmp.Ordered](ops ...Option[E]) (*List[E], error) {
	return New(cmp.Compare[E], ops...)
}

// NewFromSlice creates a new array-backed list holding a copy of the given
// slice. The provided cmp orders the elements, as in New. Optional options can
// be passed to modify the list's behavior, such as enabling concurrent safety.
func NewFromSlice[E any](cmp func(E, E) int, elements []E, ops ...Option[E]) (*List[E], error) {
	l, err := New(cmp, ops...)
	if err != nil {
		return nil, err
	}
	l.growBy(len(elements))
	copy(l.elements, elements)
	return l, nil
}

// NewFromOrderedSlice creates a new array-backed list from the given slice. It
// uses cmp.Compare[E] as the default comparison function for elements. Optional
// options can be passed to modify the list's behavior, such as enabling
// concurrent safety.
func NewFromOrderedSlice[E cmp.Ordered](elements []E, ops ...Option[E]) (*List[E], error) {
	return NewFromSlice(cmp.Compare[E], elements, ops...)
}

// Get returns the element at the given index.
func (l *List[E]) Get(index int) (E, bool) {
	if l.safe {
		l.mu.RLock()
		defer l.mu.RUnlock()
	}

	if !l.withinRange(index, false) {
		var e E
		return e, false
	}
	return l.elements[index], true
}

// Append appends specified elements to the end of the list.
func (l *List[E]) Append(el ...E) {
	if len(el) == 0 {
		return
	}
	if l.safe {
		l.mu.Lock()
		defer l.mu.Unlock()
	}

	l.append(el...)
}

func (l *List[E]) append(el ...E) {
	oldLen := len(l.elements)
	l.growBy(len(el))
	for i := range el {
		l.elements[oldLen+i] = el[i]
	}
}

// Insert inserts elements at the given index.
// If the index is the length of the list, the elements will be appended.
// If the index out of range, this function is no-op.
func (l *List[E]) Insert(index int, el ...E) {
	if len(el) == 0 {
		return
	}
	if l.safe {
		l.mu.Lock()
		defer l.mu.Unlock()
	}

	if !l.withinRange(index, true) {
		return
	}
	if index == len(l.elements) {
		l.append(el...)
		return
	}

	addLen := len(el)
	oldLen := len(l.elements)
	l.growBy(addLen)
	// move elements after index + length of "el".
	copy(l.elements[index+addLen:], l.elements[index:oldLen])
	// copy elements after index
	copy(l.elements[index:index+addLen], el)
}

// Set sets the element at the given index.
// If the index is the length of the list, the element will be appended.
// If the index out of range, this function is no-op.
func (l *List[E]) Set(index int, e E) {
	if l.safe {
		l.mu.Lock()
		defer l.mu.Unlock()
	}

	if !l.withinRange(index, true) {
		return
	}
	if index == len(l.elements) {
		l.append(e)
		return
	}
	l.elements[index] = e
}

// Remove removes every element equal to e from the list.
func (l *List[E]) Remove(e E) {
	if l.safe {
		l.mu.Lock()
		defer l.mu.Unlock()
	}

	i := 0
	for i < len(l.elements) {
		if l.cmp(e, l.elements[i]) == 0 {
			l.removeAt(i)
		} else {
			i++
		}
	}
}

// RemoveAt removes the element at the given index. If the index out of range,
// this function is no-op and returns zero value of E.
func (l *List[E]) RemoveAt(index int) E {
	if l.safe {
		l.mu.Lock()
		defer l.mu.Unlock()
	}

	return l.removeAt(index)
}

func (l *List[E]) removeAt(index int) E {
	var e E
	if !l.withinRange(index, false) {
		return e
	}
	e = l.elements[index]
	l.elements = slices.Delete(l.elements, index, index+1)
	l.shrink()
	return e
}

// Clear removes all elements from the list.
func (l *List[E]) Clear() {
	if l.safe {
		l.mu.Lock()
		defer l.mu.Unlock()
	}

	l.elements = make([]E, 0)
}

// Contains reports whether the list contains all the given elements. It
// returns true if no element is given: all of nothing is present.
func (l *List[E]) Contains(el ...E) bool {
	if len(el) == 0 {
		return true
	}

	for _, e := range el {
		if !l.contains(e) {
			return false
		}
	}
	return true
}

func (l *List[E]) contains(e E) bool {
	if l.safe {
		l.mu.RLock()
		defer l.mu.RUnlock()
	}

	for _, v := range l.elements {
		if l.cmp(v, e) == 0 {
			return true
		}
	}
	return false
}

// Values returns a slice containing all elements in the list.
// The returned slice is a copy of the internal slice,
// and modifications to it will not affect the list.
func (l *List[E]) Values() []E {
	if l.safe {
		l.mu.RLock()
		defer l.mu.RUnlock()
	}
	return slices.Clone(l.elements)
}

// IndexOf returns the index of the first occurrence of element in the list.
func (l *List[E]) IndexOf(e E) int {
	if l.safe {
		l.mu.RLock()
		defer l.mu.RUnlock()
	}

	for i, v := range l.elements {
		if l.cmp(v, e) == 0 {
			return i
		}
	}
	return -1
}

// IsEmpty reports whether the list has no elements.
func (l *List[E]) IsEmpty() bool {
	if l.safe {
		l.mu.RLock()
		defer l.mu.RUnlock()
	}
	return len(l.elements) == 0
}

// Len returns the number of elements in the list.
func (l *List[E]) Len() int {
	if l.safe {
		l.mu.RLock()
		defer l.mu.RUnlock()
	}

	return len(l.elements)
}

// Sort sorts the list by the comparison function it was built with.
func (l *List[E]) Sort() {
	if l.safe {
		l.mu.Lock()
		defer l.mu.Unlock()
	}

	if len(l.elements) < 2 {
		return
	}
	slices.SortFunc(l.elements, l.cmp)
}

// Swap swaps the elements at the given indexes.
func (l *List[E]) Swap(i, j int) {
	if l.safe {
		l.mu.Lock()
		defer l.mu.Unlock()
	}

	if l.withinRange(i, false) && l.withinRange(j, false) {
		l.elements[i], l.elements[j] = l.elements[j], l.elements[i]
	}
}

// Range calls fn on each element in the list. If fn returns false, the
// iteration stops; if fn is nil, the method does nothing.
func (l *List[E]) Range(fn func(e E) bool) {
	if fn == nil {
		return
	}
	if l.safe {
		l.mu.RLock()
		defer l.mu.RUnlock()
	}

	for _, e := range l.elements {
		if !fn(e) {
			return
		}
	}
}

func (l *List[E]) resize(len_, cap_ int) {
	newElements := make([]E, len_, cap_)
	copy(newElements, l.elements)
	l.elements = newElements
}

func (l *List[E]) growBy(n int) {
	currCap := cap(l.elements)
	newLen := len(l.elements) + n
	if newLen > currCap {
		newCap := int(growthFactor * float32(currCap))
		if newCap == 0 {
			newCap = minCap
		}
		for newCap < newLen {
			newCap = int(growthFactor * float32(newCap))
		}
		l.resize(newLen, newCap)
	} else {
		l.elements = l.elements[:newLen]
	}
}

func (l *List[E]) shrink() {
	currCap := cap(l.elements)
	if len(l.elements) <= int(shrinkFactor*float32(currCap)) {
		newCap := max(int(shrinkFactor*float32(currCap)), minCap)
		l.resize(len(l.elements), newCap)
	}
}

func (l *List[E]) withinRange(index int, allowEnd bool) bool {
	if allowEnd {
		return index >= 0 && index <= len(l.elements)
	}
	return index >= 0 && index < len(l.elements)
}
