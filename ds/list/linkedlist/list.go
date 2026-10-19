// Package linkedlist provides a doubly-linked list with a front and a back.
// Its nodes are handles: they carry the values, can be walked from one to the
// next, and are what the list's methods take to insert next to a node or to
// remove one, while the links between them stay the list's own, see Node.
//
// Invariants:
// - the front node has no previous node and the back node no next one
// - every node between them has both
// - an empty list has neither a front nor a back node
// - every node reachable from the front is on this list, and Len counts them
package linkedlist

import (
	"reflect"
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/ds/types"
)

// ErrMergeWithSelf is returned by Merge and MergeSorted when a list is asked
// to merge with itself: emptying the other list afterwards would empty the
// result.
var ErrMergeWithSelf = errors.New("cannot merge a list with itself")

// List represents a doubly-linked list.
type List[V any] struct {
	head, tail *Node[V]
	count      int
	mu         types.Locker
	safe       bool
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

// pushBackNode links n in at the back of the list and puts it on the list.
func (l *List[V]) pushBackNode(n *Node[V]) *Node[V] {
	n.prev = l.tail
	n.next = nil
	n.list = l
	// If l.tail is nil, the list is empty, and 'n' becomes the first node.
	// Both l.head and l.tail will point to 'n'.
	if l.tail != nil {
		l.tail.next = n
	} else {
		l.head = n
	}
	l.tail = n
	l.count++

	return n
}

// PushFront adds a new node with value v to the beginning of the list.
func (l *List[V]) PushFront(v V) *Node[V] {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.pushFrontNode(&Node[V]{Value: v})
}

// pushFrontNode links n in at the front of the list and puts it on the list.
func (l *List[V]) pushFrontNode(n *Node[V]) *Node[V] {
	n.prev = nil
	n.next = l.head
	n.list = l
	// l.head is nil means the list is empty, node 'n' is the first node,
	// so l.head and l.tail will point to 'n'.
	if l.head != nil {
		l.head.prev = n
	} else {
		l.tail = n
	}
	l.head = n
	l.count++

	return n
}

// InsertAfter inserts value v after the node n and returns the new node. It
// returns nil, changing nothing, when n is nil or not on this list.
func (l *List[V]) InsertAfter(n *Node[V], v V) *Node[V] {
	if n == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if n.list != l {
		return nil
	}
	next := &Node[V]{Value: v, prev: n, next: n.next, list: l}
	// if n.next is nil, means 'n' is the list tail node.
	if n.next != nil {
		n.next.prev = next
	} else {
		l.tail = next
	}
	n.next = next
	l.count++

	return next
}

// InsertBefore inserts value v before the node n and returns the new node.
// It returns nil, changing nothing, when n is nil or not on this list.
func (l *List[V]) InsertBefore(n *Node[V], v V) *Node[V] {
	if n == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if n.list != l {
		return nil
	}
	prev := &Node[V]{Value: v, prev: n.prev, next: n, list: l}
	// if n.prev is nil, means 'n' is the list head node.
	if n.prev != nil {
		n.prev.next = prev
	} else {
		l.head = prev
	}
	n.prev = prev
	l.count++

	return prev
}

// Remove removes the node n from the list when n is on it, and returns the
// value of n either way, the zero value for a nil n. A node off its list,
// removed already or on another list, leaves this list unchanged.
func (l *List[V]) Remove(n *Node[V]) V {
	var v V
	if n == nil {
		return v
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if n.list != l {
		return n.Value
	}
	return l.remove(n)
}

// remove unlinks n, a node on the list, and takes it off the list: its links
// are cleared so it holds on to no other node and no list.
func (l *List[V]) remove(n *Node[V]) V {
	// n.next is nil means 'n' is the list tail node.
	if n.next != nil {
		n.next.prev = n.prev
	} else {
		l.tail = n.prev
	}
	// n.prev is nil means 'n' is the list head node.
	if n.prev != nil {
		n.prev.next = n.next
	} else {
		l.head = n.next
	}
	n.prev, n.next, n.list = nil, nil, nil
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
	return l.remove(l.head)
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
	return l.remove(l.tail)
}

// Slice returns a slice containing all values in the list,
// in the order they appear.
func (l *List[V]) Slice() []V {
	l.mu.RLock()
	defer l.mu.RUnlock()

	s := make([]V, 0, l.count)
	for n := l.head; n != nil; n = n.next {
		s = append(s, n.Value)
	}
	return s
}

// Range calls the given function for each element in the list,
// in order from front to back.
func (l *List[V]) Range(fn func(v V) bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for n := l.head; n != nil; n = n.next {
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

	for n := l.head; n != nil; n = n.next {
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
	curr := l.head
	l.head, l.tail = l.tail, l.head
	for curr != nil {
		next := curr.next
		curr.next, curr.prev = curr.prev, curr.next
		curr = next
	}
}

// Merge appends all elements from other list to the end of current list:
// the nodes of other move onto l, the handles to them stay valid there. The
// other list is emptied after the merge; a nil other merges nothing. It
// fails with ErrMergeWithSelf, changing nothing, when other is l itself.
func (l *List[V]) Merge(other *List[V]) error {
	if other == l {
		return ErrMergeWithSelf
	}
	if other == nil {
		return nil
	}

	unlock := lockBoth(l, other)
	defer unlock()

	if other.isEmpty() {
		return nil
	}
	for n := other.head; n != nil; n = n.next {
		n.list = l
	}
	if l.isEmpty() {
		l.head = other.head
		l.tail = other.tail
		l.count = other.count
	} else {
		l.tail.next = other.head
		other.head.prev = l.tail
		l.tail = other.tail
		l.count += other.count
	}

	other.head = nil
	other.tail = nil
	other.count = 0
	return nil
}

// MergeSorted merges two lists into one sorted list using the provided cmp function.
// The compare function should return:
//   - negative value if a < b.
//   - zero if a = b.
//   - positive value if a > b.
//
// Both input list can be unsorted, the resulting list will be sorted, also
// when other is empty or nil; equal values keep their order, those of l
// first. The nodes of both lists end up on l, so the handles to them stay
// valid there, and the other list is emptied. It fails, changing neither
// list, with types.ErrComparisonNil when cmp is nil and with ErrMergeWithSelf
// when other is l itself.
func (l *List[V]) MergeSorted(other *List[V], cmp func(V, V) int) error {
	if cmp == nil {
		return types.ErrComparisonNil
	}
	if other == l {
		return ErrMergeWithSelf
	}

	var unlock func()
	if other == nil {
		l.mu.Lock()
		unlock = l.mu.Unlock
	} else {
		unlock = lockBoth(l, other)
	}
	defer unlock()

	size := l.count
	if other != nil {
		size += other.count
	}
	nodes := make([]*Node[V], 0, size)
	for curr := l.head; curr != nil; curr = curr.next {
		nodes = append(nodes, curr)
	}
	if other != nil {
		for curr := other.head; curr != nil; curr = curr.next {
			nodes = append(nodes, curr)
		}
		other.head = nil
		other.tail = nil
		other.count = 0
	}
	slices.SortStableFunc(nodes, func(a, b *Node[V]) int { return cmp(a.Value, b.Value) })
	l.head = nil
	l.tail = nil
	l.count = 0
	for _, n := range nodes {
		l.pushBackNode(n)
	}
	return nil
}

// lockBoth write-locks l and other in the order of their addresses, which is
// the same order whichever of the two lists a merge starts from: two lists
// merging into each other at once then never each hold the lock the other
// waits for. It returns the function that unlocks both.
func lockBoth[V any](l, other *List[V]) (unlock func()) {
	first, second := l, other
	if reflect.ValueOf(first).Pointer() > reflect.ValueOf(second).Pointer() {
		first, second = second, first
	}
	first.mu.Lock()
	second.mu.Lock()
	return func() {
		second.mu.Unlock()
		first.mu.Unlock()
	}
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
	for n := l.head; n != nil; n = n.next {
		newList.pushBackNode(&Node[V]{Value: n.Value})
	}
	return newList, nil
}

// Front returns the first node of the list, or nil when the list is empty.
func (l *List[V]) Front() *Node[V] {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.head
}

// Back returns the last node of the list, or nil when the list is empty.
func (l *List[V]) Back() *Node[V] {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.tail
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

// Clear removes all nodes from the list; each is then on no list.
func (l *List[V]) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()

	curr := l.head
	for curr != nil {
		next := curr.next
		curr.prev, curr.next, curr.list = nil, nil, nil
		curr = next
	}

	l.head = nil
	l.tail = nil
	l.count = 0
}
