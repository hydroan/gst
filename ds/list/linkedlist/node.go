package linkedlist

// Node is a node in the doubly-linked list.
type Node[V any] struct {
	Value      V
	Prev, Next *Node[V]
}

// Each calls 'fn' on every element from this node onward in the list.
func (n *Node[V]) Each(fn func(val V)) {
	node := n
	for node != nil {
		fn(node.Value)
		node = node.Next
	}
}

// EachReverse calls 'fn' on every element from this node backward in the list.
func (n *Node[V]) EachReverse(fn func(val V)) {
	node := n
	for node != nil {
		fn(node.Value)
		node = node.Prev
	}
}

// EachNode calls 'fn' on every node from this node onward in the list.
func (n *Node[V]) EachNode(fn func(n *Node[V])) {
	node := n
	for node != nil {
		fn(node)
		node = node.Next
	}
}

// EachReverseNode calls 'fn' on every node from this node backward in the list.
func (n *Node[V]) EachReverseNode(fn func(n *Node[V])) {
	node := n
	for node != nil {
		fn(node)
		node = node.Prev
	}
}
