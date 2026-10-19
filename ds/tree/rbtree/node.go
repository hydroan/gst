package rbtree

import "fmt"

type color bool

const (
	black, red color = true, false
)

// The terminal escape codes String colors the nodes with.
const (
	reset   = "\033[0m"
	redBg   = "\033[41;37m"
	blackBg = "\033[40;37m"
)

func (c color) String() string {
	switch c {
	case black:
		return "black"
	case red:
		return "red"
	default:
		return "unknown"
	}
}

func (c color) symbol() string {
	switch c {
	case black:
		return "B"
	case red:
		return "R"
	default:
		return "-"
	}
}

// node represents a node in red-black tree.
type node[K comparable, V any] struct {
	Key   K
	Value V
	color color

	Left   *node[K, V]
	Right  *node[K, V]
	Parent *node[K, V]
}

// Color returns a string representation of the node's color.
func (n *node[K, V]) Color() string { return n.color.String() }

// Size returns the number of elements in the subtree rooted at n.
func (n *node[K, V]) Size() int {
	if n == nil {
		return 0
	}
	size := 1
	if n.Left != nil {
		size += n.Left.Size()
	}
	if n.Right != nil {
		size += n.Right.Size()
	}
	return size
}

// String returns a string representation of the node's key.
func (n *node[K, V]) String() string {
	return fmt.Sprintf("%v", n.Key)
}

// grandparent returns the grandparent node of n.
func (n *node[K, V]) grandparent() *node[K, V] {
	if n != nil && n.Parent != nil {
		return n.Parent.Parent
	}
	return nil
}

// sibling returns the sibling node of n.
func (n *node[K, V]) sibling() *node[K, V] {
	if n == nil || n.Parent == nil {
		return nil
	}
	if n == n.Parent.Left {
		return n.Parent.Right
	}
	return n.Parent.Left
}

// uncle returns the uncle node of n.
func (n *node[K, V]) uncle() *node[K, V] {
	if n == nil || n.Parent == nil || n.Parent.Parent == nil {
		return nil
	}
	return n.Parent.sibling()
}

// maximum returns the maximum node in the subtree rooted at n.
func (n *node[K, V]) maximum() *node[K, V] {
	if n == nil {
		return nil
	}
	for n.Right != nil {
		n = n.Right
	}
	return n
}
