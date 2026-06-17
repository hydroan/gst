package avltree

import "fmt"

type Node[K comparable, V any] struct {
	Key      K
	Value    V
	Parent   *Node[K, V]
	Children [2]*Node[K, V]
	b        int
}

// Size returns the number of nodes in the subtree rooted at current node.
func (n *Node[K, V]) Size() int {
	if n == nil {
		return 0
	}
	size := 1
	if n.Children[0] != nil {
		size += n.Children[0].Size()
	}
	if n.Children[1] != nil {
		size += n.Children[1].Size()
	}
	return size
}

// Prev returns the previous node in an inorder traversal.
func (n *Node[K, V]) Prev() *Node[K, V] {
	return n.walk(0)
}

// Next returns the next node in an inorder traversal.
func (n *Node[K, V]) Next() *Node[K, V] {
	return n.walk(1)
}

// String returns a string representation of the node's key.
func (n *Node[K, V]) String() string {
	return fmt.Sprintf("%v", n.Key)
}

func (n *Node[K, V]) walk(a int) *Node[K, V] {
	if n == nil {
		return nil
	}
	if n.Children[a] != nil {
		n = n.Children[a]
		for n.Children[a^1] != nil {
			n = n.Children[a^1]
		}
		return n
	}
	p := n.Parent
	for p != nil && p.Children[a] == n {
		n = p
		p = p.Parent
	}
	return p
}
