// Package linkedlist provides an implementation of a doubly-linked list with a front
// and back. The individual node of the list are publicly exposed so that the
// user can have fine-grained control over the list.
//
// Invariants:
// - Head.Prev must be nil
// - Tail.Next must be nil
// - All internal nodes must have non-nil Prev and Next
// - An empty list has both Head and Tail as ni
package linkedlist

import (
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/ds/types"
)

var ErrNilCmp = errors.New("nil comparator")

// List represents a doubly-linked list.
type List[V any] struct {
	Head, Tail *Node[V]
	count      int
	mu         types.Locker
	safe       bool
	sorted     bool
	cmp        func(V, V) int
}

// New creates and returns an empty doubly-linked list.
func New[V any](ops ...Option[V]) (*List[V], error) {
	l := new(List[V])
	l.mu = types.FakeLocker{}
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

// NewFromSlice creates a doubly-linked list from a slice.
func NewFromSlice[V any](s []V, ops ...Option[V]) (*List[V], error) {
	l, err := New(ops...)
	if err != nil {
		return nil, err
	}
	for _, v := range s {
		l.PushBack(v)
	}
	return l, nil
}

// PushBack adds a new node with value v to the end of the list.
// Returns a pointer to the newly added node.
func (l *List[V]) PushBack(v V) *Node[V] {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.pushBackNode(&Node[V]{Value: v})
}

// PushBackNode adds the given node to the end of the list.
// The node should not be part of another list.
func (l *List[V]) PushBackNode(n *Node[V]) *Node[V] {
	if n == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.pushBackNode(n)
}

func (l *List[V]) pushBackNode(n *Node[V]) *Node[V] {
	n.Prev = l.Tail
	n.Next = nil
	// If l.Tail is nil, the list is empty, and 'n' becomes the first node.
	// Both l.Head and l.Tail will point to 'n'.
	if l.Tail != nil {
		l.Tail.Next = n
	} else {
		l.Head = n
	}
	l.Tail = n
	l.count++

	return n
}

// PushFront adds a new node with value v to the beginning of the list.
func (l *List[V]) PushFront(v V) *Node[V] {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.pushFrontNode(&Node[V]{Value: v})
}

// PushFrontNode adds the given node to the beginning of the list.
// The node should not be part of another list.
func (l *List[V]) PushFrontNode(n *Node[V]) *Node[V] {
	if n == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.pushFrontNode(n)
}

func (l *List[V]) pushFrontNode(n *Node[V]) *Node[V] {
	n.Prev = nil
	n.Next = l.Head
	// l.Head is nil means the list is empty, node 'n' is the first node.
	// so l.Head and l.Tail will pointer to 'n'.
	if l.Head != nil {
		l.Head.Prev = n
	} else {
		l.Tail = n
	}
	l.Head = n
	l.count++

	return n
}

// InsertAfter inserts value v after the given node.
// If the node is nil, the operation is a no-op.
// Returns the inserted node.
func (l *List[V]) InsertAfter(n *Node[V], v V) *Node[V] {
	if n == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.insertAfterNode(n, &Node[V]{Value: v})
}

// InsertAfterNode inserts node 'next' after the given node.
func (l *List[V]) InsertAfterNode(n *Node[V], next *Node[V]) *Node[V] {
	if n == nil || next == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.insertAfterNode(n, next)
}

func (l *List[V]) insertAfterNode(n *Node[V], next *Node[V]) *Node[V] {
	next.Prev = n
	next.Next = n.Next
	// if n.Next is nil, means 'n' is the list Tail Node.
	if n.Next != nil {
		n.Next.Prev = next
	} else {
		l.Tail = next
	}
	n.Next = next

	l.count++

	return next
}

// InsertBefore inserts value v before the given node.
// If the node is nil, the operation is a no-op.
// Returns the inserted node.
func (l *List[V]) InsertBefore(n *Node[V], v V) *Node[V] {
	if n == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.insertBeforeNode(n, &Node[V]{Value: v})
}

// InsertBeforeNode inserts node 'prev' before the given node.
func (l *List[V]) InsertBeforeNode(n *Node[V], prev *Node[V]) *Node[V] {
	if n == nil || prev == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.insertBeforeNode(n, prev)
}

func (l *List[V]) insertBeforeNode(n *Node[V], prev *Node[V]) *Node[V] {
	prev.Prev = n.Prev
	prev.Next = n
	// if n.Prev is nil, means 'n' is the list Head Node.
	if n.Prev != nil {
		n.Prev.Next = prev
	} else {
		l.Head = prev
	}
	n.Prev = prev

	l.count++

	return prev
}

// Remove removes the given node from the list.
// If the node is nil, it does nothing and returns the zero value.
func (l *List[V]) Remove(n *Node[V]) V {
	var v V
	if n == nil {
		return v
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.remove(n)
}

func (l *List[V]) remove(n *Node[V]) V {
	// n.Next is nil means 'n' is the list Tail Node.
	if n.Next != nil {
		n.Next.Prev = n.Prev
	} else {
		l.Tail = n.Prev
	}
	// n.Prev is nil means 'n' is the list Head Node.
	if n.Prev != nil {
		n.Prev.Next = n.Next
	} else {
		l.Head = n.Next
	}

	l.count--

	return n.Value
}

// PopFront removes and returns the first node of the list.
// Returns nil if the list is empty.
func (l *List[V]) PopFront() V {
	l.mu.Lock()
	defer l.mu.Unlock()

	var v V
	if l.isEmpty() {
		return v
	}
	n := l.Head
	return l.remove(n)
}

// PopBack removes and returns the last node of the list.
// Returns nil if the list is empty.
func (l *List[V]) PopBack() V {
	l.mu.Lock()
	defer l.mu.Unlock()

	var v V
	if l.isEmpty() {
		return v
	}
	n := l.Tail
	return l.remove(n)
}

// Slice returns a slice containing all values in the list,
// in the order they appear.
func (l *List[V]) Slice() []V {
	l.mu.RLock()
	defer l.mu.RUnlock()

	s := make([]V, 0, l.count)
	for n := l.Head; n != nil; n = n.Next {
		s = append(s, n.Value)
	}
	return s
}

// Range calls the given function for each element in the list,
// in order from front to back.
func (l *List[V]) Range(fn func(v V) bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for n := l.Head; n != nil; n = n.Next {
		if !fn(n.Value) {
			return
		}
	}
}

// Find searches for a node with the given value using the provided equality function.
// Returns nil if not found.
func (l *List[V]) Find(v V, equal func(V, V) bool) *Node[V] {
	if equal == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()

	for n := l.Head; n != nil; n = n.Next {
		if equal(n.Value, v) {
			return n
		}
	}
	return nil
}

// Reverse reverses the order of nodes in the list in-place.
// Empty list and single-node list remain unchanged.
func (l *List[V]) Reverse() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.count <= 1 {
		return
	}
	curr := l.Head
	l.Head, l.Tail = l.Tail, l.Head
	for curr != nil {
		next := curr.Next
		curr.Next, curr.Prev = curr.Prev, curr.Next
		curr = next
	}
}

// Merge appends all elements from other list to the end of current list.
// The other list is emptied after the merge.
func (l *List[V]) Merge(other *List[V]) {
	if other == nil {
		return
	}
	if other.isEmpty() {
		return
	}

	l.mu.Lock()
	other.mu.Lock()
	defer l.mu.Unlock()
	defer other.mu.Unlock()

	if l.isEmpty() {
		l.Head = other.Head
		l.Tail = other.Tail
		l.count = other.count
	} else {
		l.Tail.Next = other.Head
		other.Head.Prev = l.Tail
		l.Tail = other.Tail
		l.count += other.count
	}

	other.Head = nil
	other.Tail = nil
	other.count = 0
}

// MergeSorted merges two lists into one sorted list using the provided cmp function.
// The compare function should return:
//   - negative value if a < b.
//   - zero if a = b.
//   - positive value if a > b.
//
// Both input list can be unsorted, the resulting list will be sorted.
// The other list is emptied after the merge.
func (l *List[V]) MergeSorted(other *List[V], cmp func(V, V) int) {
	if other == nil {
		return
	}
	if other.isEmpty() {
		return
	}
	if cmp == nil {
		return
	}

	l.mu.Lock()
	other.mu.Lock()
	defer l.mu.Unlock()
	defer other.mu.Unlock()

	merged := make([]V, 0, l.count+other.count)
	curr := l.Head
	for curr != nil {
		merged = append(merged, curr.Value)
		curr = curr.Next
	}
	curr = other.Head
	for curr != nil {
		merged = append(merged, curr.Value)
		curr = curr.Next
	}
	slices.SortFunc(merged, cmp)
	l.Head = nil
	l.Tail = nil
	l.count = 0
	for _, v := range merged {
		l.pushBackNode(&Node[V]{Value: v})
	}
	other.Head = nil
	other.Tail = nil
	other.count = 0
}

// Clone returns a deep copy of the list.
// The new list contains new nodes with the same values.
func (l *List[V]) Clone() (*List[V], error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	newList, err := New[V]()
	if err != nil {
		return nil, err
	}
	for n := l.Head; n != nil; n = n.Next {
		newList.pushBackNode(&Node[V]{Value: n.Value})
	}
	return newList, nil
}

// Len returns the number of nodes in the list.
func (l *List[V]) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.count
}

// IsEmpty reports whether the list has no elements.
func (l *List[V]) IsEmpty() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.isEmpty()
}

func (l *List[V]) isEmpty() bool {
	return l.count == 0
}

// Clear removes all nodes from the list.
func (l *List[V]) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()

	curr := l.Head
	for curr != nil {
		next := curr.Next
		curr.Next = nil
		curr.Prev = nil
		curr = next
	}

	l.Head = nil
	l.Tail = nil
	l.count = 0
}
