package linkedlist

// Node is an element of a List: it carries a value and is the handle the
// list's methods take to insert next to it or to remove it. The links between
// nodes belong to the list, which keeps them and its length consistent; Next
// and Prev read them. A node is on the list that created it, or on the one a
// merge moved it to, until it is removed, and on no list afterwards; a list's
// methods leave a node that is not on it alone.
//
// Next, Prev and the Each methods read the links without the list's lock: on
// a list shared between goroutines, walk it with List.Range or List.Slice.
type Node[V any] struct {
	Value V

	prev, next *Node[V]
	list       *List[V]
}

// Next returns the node after n, or nil when n is the last node of its list
// or is on no list.
func (n *Node[V]) Next() *Node[V] { return n.next }

// Prev returns the node before n, or nil when n is the first node of its
// list or is on no list.
func (n *Node[V]) Prev() *Node[V] { return n.prev }

// Each calls 'fn' on every element from this node onward in the list.
func (n *Node[V]) Each(fn func(val V)) {
	for node := n; node != nil; node = node.next {
		fn(node.Value)
	}
}

// EachReverse calls 'fn' on every element from this node backward in the list.
func (n *Node[V]) EachReverse(fn func(val V)) {
	for node := n; node != nil; node = node.prev {
		fn(node.Value)
	}
}

// EachNode calls 'fn' on every node from this node onward in the list.
func (n *Node[V]) EachNode(fn func(n *Node[V])) {
	for node := n; node != nil; node = node.next {
		fn(node)
	}
}

// EachReverseNode calls 'fn' on every node from this node backward in the list.
func (n *Node[V]) EachReverseNode(fn func(n *Node[V])) {
	for node := n; node != nil; node = node.prev {
		fn(node)
	}
}
