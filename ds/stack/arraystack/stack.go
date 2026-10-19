package arraystack

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hydroan/gst/ds/types"
)

// Stack represents a stack backed by a slice.
// The stack provides typical LIFO(last-in, first-out) behavior.
type Stack[E any] struct {
	elements []E
	safe     bool
	mu       types.Locker
}

// New creates and initializes a empty stack.
// Options can be provided to customize the stack's properties (e.g., thread safety).
func New[E any](ops ...Option[E]) (*Stack[E], error) {
	s := &Stack[E]{elements: make([]E, 0), mu: types.FakeLocker{}}
	for _, op := range ops {
		if op == nil {
			continue
		}
		if err := op(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// NewFromSlice creates and initializes a stack from the provided slice, the
// last element of the slice on top.
// Options can be provided to customize the stack's properties (e.g., thread safety).
func NewFromSlice[E any](slice []E, ops ...Option[E]) (*Stack[E], error) {
	s, err := New(ops...)
	if err != nil {
		return nil, err
	}
	s.elements = append(s.elements, slice...)
	return s, nil
}

// NewFromMapKeys creates and initializes a stack from the provided map keys.
// Options can be provided to customize the stack's properties (e.g., thread safety).
// Returns an empty stack if the provided map is nil or empty.
func NewFromMapKeys[K comparable, V any](m map[K]V, ops ...Option[K]) (*Stack[K], error) {
	s, err := New(ops...)
	if err != nil {
		return nil, err
	}
	for k := range m {
		s.elements = append(s.elements, k)
	}
	return s, nil
}

// NewFromMapValues creates a stack from the provided map values.
// Options can be provided to customize the stack's properties (e.g., thread safety).
// Returns an empty stack if the provided map is nil or empty.
func NewFromMapValues[K comparable, V any](m map[K]V, ops ...Option[V]) (*Stack[V], error) {
	s, err := New(ops...)
	if err != nil {
		return nil, err
	}
	for _, v := range m {
		s.elements = append(s.elements, v)
	}
	return s, nil
}

// Push adds an element to the top of the stack.
// The stack's size increases by one.
func (s *Stack[E]) Push(e E) {
	if s.safe {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

	s.elements = append(s.elements, e)
}

// Pop removes and returns the top element of the stack.
// Returns the zero value of E and false if the stack is empty.
func (s *Stack[E]) Pop() (E, bool) {
	if s.safe {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

	var zero E
	last := len(s.elements) - 1
	if last < 0 {
		return zero, false
	}
	e := s.elements[last]
	// Clearing the slot lets the backing array drop its reference to e.
	s.elements[last] = zero
	s.elements = s.elements[:last]
	return e, true
}

// Peek returns the top element of the stack without removing it.
// Returns the zero value of E and false if the stack is empty.
func (s *Stack[E]) Peek() (E, bool) {
	if s.safe {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}

	if len(s.elements) == 0 {
		var zero E
		return zero, false
	}
	return s.elements[len(s.elements)-1], true
}

// IsEmpty reports whether the stack has no elements.
func (s *Stack[E]) IsEmpty() bool {
	if s.safe {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}

	return len(s.elements) == 0
}

// Len returns the number of elements currently in the stack.
func (s *Stack[E]) Len() int {
	if s.safe {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}

	return len(s.elements)
}

// Values returns all elements in the stack in LIFO(last-in, first-out) order.
// If the stack is empty, it returns an empty slice (not nil).
func (s *Stack[E]) Values() []E {
	if s.safe {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}

	el := make([]E, len(s.elements))
	copy(el, s.elements)
	slices.Reverse(el)
	return el
}

// Clear removes all elements from the stack.
func (s *Stack[E]) Clear() {
	if s.safe {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

	s.elements = make([]E, 0)
}

// Clone returns a deep copy of the stack.
func (s *Stack[E]) Clone() *Stack[E] {
	if s.safe {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}

	clone, _ := NewFromSlice(s.elements, s.options()...)
	return clone
}

func (s *Stack[E]) options() []Option[E] {
	ops := make([]Option[E], 0)
	if s.safe {
		ops = append(ops, WithSafe[E]())
	}
	return ops
}

// String returns a string representation of the stack.
func (s *Stack[E]) String() string {
	if s.safe {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}

	items := make([]string, 0, len(s.elements))
	for _, e := range slices.Backward(s.elements) {
		items = append(items, fmt.Sprintf("%v", e))
	}
	return fmt.Sprintf("stack:{%s}", strings.Join(items, ", "))
}
