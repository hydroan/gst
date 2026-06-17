package linkedstack

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hydroan/gst/ds/list/linkedlist"
	"github.com/hydroan/gst/ds/types"
)

// Stack represents a stack based on linkedlist..
// The stack provides typical LIFO(last-in, first-out) behavior.
type Stack[E any] struct {
	list *linkedlist.List[E]
	safe bool
	mu   types.Locker
}

// New creates and initializes a empty stack.
// Options can be provided to customize the stack's properties (e.g., thread safety).
func New[E any](ops ...Option[E]) (s *Stack[E], err error) {
	s = &Stack[E]{mu: types.FakeLocker{}}
	for _, op := range ops {
		if op == nil {
			continue
		}
		if err = op(s); err != nil {
			return nil, err
		}
	}
	// internal list alway concurrent unsafe.
	s.list, err = linkedlist.New[E]()
	if err != nil {
		return nil, err
	}
	return s, nil
}

// NewFromSlice creates and initializes a stack from the provided slice.
// Options can be provided to customize the stack's properties (e.g., thread safety).
func NewFromSlice[E any](slice []E, ops ...Option[E]) (*Stack[E], error) {
	s, err := New(ops...)
	if err != nil {
		return nil, err
	}
	if len(slice) > 0 {
		for _, e := range slice {
			s.list.PushBack(e)
		}
	}
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
	if len(m) > 0 {
		for k := range m {
			s.list.PushBack(k)
		}
	}
	return s, nil
}

// NewFromMapValues creates a stack from the provided map values.
// Options can be provided to customize the stack's properties (e.g., thread safety).
// Returns an empty stack if the provided map is nil or empty.
func NewFromMapValues[K comparable, V any](cmp func(V, V) int, m map[K]V, ops ...Option[V]) (*Stack[V], error) {
	s, err := New(ops...)
	if err != nil {
		return nil, err
	}
	if len(m) > 0 {
		for _, v := range m {
			s.list.PushBack(v)
		}
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

	s.list.PushBack(e)
}

// Pop removes and returns the top element of the stack.
// Returns the zero value of E and false if the stack is empty.
func (s *Stack[E]) Pop() (E, bool) {
	if s.safe {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

	var e E
	if s.list.Len() == 0 {
		return e, false
	}
	return s.list.PopBack(), true
}

// Peek returns the top element of the stack without removing it.
// Returns the zero value of E and false if the stack is empty.
func (s *Stack[E]) Peek() (E, bool) {
	if s.safe {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}

	var e E
	if s.list.IsEmpty() {
		return e, false
	}
	return s.list.Tail.Value, true
}

// IsEmpty reports whether the stack has no elements.
func (s *Stack[E]) IsEmpty() bool {
	if s.safe {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}

	return s.list.IsEmpty()
}

// Len returns the number of elements currently in the stack.
func (s *Stack[E]) Len() int {
	if s.safe {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}

	return s.list.Len()
}

// Values returns all elements in the stack in LIFO(last-in, first-out) order.
// If the stack is empty, it returns an empty slice (not nil).
func (s *Stack[E]) Values() []E {
	if s.safe {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}

	el := s.list.Slice()
	slices.Reverse(el)
	return el
}

// Clear removes all elements from the stack.
func (s *Stack[E]) Clear() {
	if s.safe {
		s.mu.Lock()
		defer s.mu.Unlock()
	}

	s.list.Clear()
}

// Clone returns a deep copy of the stack.
func (s *Stack[E]) Clone() *Stack[E] {
	if s.safe {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}

	clone, _ := NewFromSlice(s.list.Slice(), s.options()...)
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

	el := s.list.Slice()
	slices.Reverse(el)
	items := make([]string, 0, s.list.Len())
	for _, e := range el {
		items = append(items, fmt.Sprintf("%v", e))
	}
	return fmt.Sprintf("stack:{%s}", strings.Join(items, ", "))
}
