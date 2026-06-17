package splaytree

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/hydroan/gst/ds/types"
)

const (
	vertical = "│   "
	space    = "    "
	tailMark = "╰── "
	headMark = "╭── "
)

type Node[K comparable, V any] struct {
	Key      K
	Value    V
	Parent   *Node[K, V]
	Children [2]*Node[K, V]
}

func (n *Node[K, V]) String() string {
	return fmt.Sprintf("%v", n.Key)
}

// Tree represents a generic splay tree.
// It support keys of any comparable type and value of any type.
// The tree use a custom comparison function to matain order.
type Tree[K comparable, V any] struct {
	root *Node[K, V]
	cmp  func(K, K) int
	size int

	safe       bool
	mu         types.Locker
	nodeFormat string
}

// New creates and returns a splay tree.
// The provided function "cmp" is determines the order of the keys.
func New[K comparable, V any](cmp func(K, K) int, ops ...Option[K, V]) (*Tree[K, V], error) {
	if cmp == nil {
		return nil, types.ErrEqualNil
	}
	t := &Tree[K, V]{cmp: cmp, mu: types.FakeLocker{}}
	for _, op := range ops {
		if op == nil {
			continue
		}
		if err := op(t); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// NewOrderedKeys creates and returns a splay tree.
// It use the cmp.Compare[K] as the default comparison function.
// This is suitable for types that implement the cmp.Ordered interface,
// such as int, float64 and string
func NewOrderedKeys[K cmp.Ordered, V any](ops ...Option[K, V]) (*Tree[K, V], error) {
	return New(cmp.Compare[K], ops...)
}

// NewFromSlice creates and returns a splay tree from a given slice.
// It use the cmp.Compare[K] as the default comparison function.
func NewFromSlice[V any](slice []V, ops ...Option[int, V]) (*Tree[int, V], error) {
	t, err := NewOrderedKeys(ops...)
	if err != nil {
		return nil, err
	}
	for i, v := range slice {
		t.Put(i, v)
	}
	return t, nil
}

// NewFromMap creates and returns a splay tree from a given map.
// The provided function "cmp" is determines the order of the keys.
func NewFromMap[K comparable, V any](m map[K]V, cmp func(K, K) int, ops ...Option[K, V]) (*Tree[K, V], error) {
	t, err := New(cmp, ops...)
	if err != nil {
		return nil, err
	}
	for k, v := range m {
		t.Put(k, v)
	}
	return t, nil
}

// NewFromOrderedMap creates and returns a splay tree from a given map.
// It uses cmp.Compare[K] as the default comparison function,
// which is suitable for types that implement the cmp.Ordered interface, such as int, float64, and string.
func NewFromOrderedMap[K cmp.Ordered, V any](m map[K]V, ops ...Option[K, V]) (*Tree[K, V], error) {
	t, err := NewOrderedKeys(ops...)
	if err != nil {
		return nil, err
	}
	for k, v := range m {
		t.Put(k, v)
	}
	return t, nil
}

// Put inserts or updates a key-value pair in the splay tree
func (t *Tree[K, V]) Put(key K, val V) {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	t.put(key, val)
}

func (t *Tree[K, V]) put(key K, val V) {
	if t.root == nil {
		t.root = &Node[K, V]{Key: key, Value: val, Children: [2]*Node[K, V]{}}
		t.size++
		return
	}
	curr := t.root
	var parent *Node[K, V]
	var isRight bool
	for curr != nil {
		parent = curr
		res := t.cmp(key, curr.Key)
		switch {
		case res == 0:
			curr.Value = val
			t.splay(curr)
			return
		case res < 0:
			isRight = false
			curr = curr.Children[0]
		case res > 0:
			isRight = true
			curr = curr.Children[1]
		}
	}
	newNode := &Node[K, V]{Key: key, Value: val, Parent: parent, Children: [2]*Node[K, V]{}}
	if isRight {
		parent.Children[1] = newNode
	} else {
		parent.Children[0] = newNode
	}
	t.size++
	t.splay(newNode)
}

// Get retrieves the value associated with the given key.
// It returns the value and true if the key exists,
// otherwise returns the zero value and false.
// The accessed node will be splayed to the root if found.
func (t *Tree[K, V]) Get(key K) (V, bool) {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	if t.root == nil {
		var zero V
		return zero, false
	}
	return t.get(key)
}

func (t *Tree[K, V]) get(key K) (V, bool) {
	n := t.root
	for n != nil {
		res := t.cmp(key, n.Key)
		if res == 0 {
			break
		} else if res < 0 {
			if n.Children[0] == nil {
				break
			}
			n = n.Children[0]
		} else {
			if n.Children[1] == nil {
				break
			}
			n = n.Children[1]
		}
	}

	t.splay(n)
	if t.cmp(key, t.root.Key) == 0 {
		return t.root.Value, true
	}
	var zero V
	return zero, false
}

// Delete removes the node with the given key from the tree.
// It returns the value associated with the key and true if found,
// otherwise returns the zero value and false.
// The operation maintains the BST property by promoting the maximum node
// of the left subtree when deleting a node with two children.
func (t *Tree[K, V]) Delete(key K) (V, bool) {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	if t.root == nil {
		var zero V
		return zero, false
	}

	if _, found := t.get(key); !found {
		var zero V
		return zero, false
	}

	// target node is now at root after splay
	oldRoot := t.root
	value := oldRoot.Value

	// case 1: leaf node
	if oldRoot.Children[0] == nil && oldRoot.Children[1] == nil {
		t.root = nil
		t.size--
		return value, true
	}

	// case 2: single child
	if oldRoot.Children[0] == nil {
		t.root = oldRoot.Children[1]
		t.root.Parent = nil
		t.size--
		return value, true
	}
	if oldRoot.Children[1] == nil {
		t.root = oldRoot.Children[0]
		t.root.Parent = nil
		t.size--
		return value, true
	}

	// case 3: two children
	right := oldRoot.Children[1]
	t.root = oldRoot.Children[0]
	// find and splay the max node in left subtree
	n := t.root
	for n.Children[1] != nil {
		n = n.Children[1]
	}
	t.splay(n)
	t.root.Children[1] = right
	right.Parent = t.root
	t.size--
	return value, true
}

// Size returns the current number of nodes in the tree.
func (t *Tree[K, V]) Size() int {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	return t.size
}

// IsEmpty returns true if the tree contains no elements, false otherwise.
func (t *Tree[K, V]) IsEmpty() bool {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	return t.size == 0
}

// Clear removes all elements from the tree.
func (t *Tree[K, V]) Clear() {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	t.root = nil
	t.size = 0
}

// Keys returns a sorted slice of all keys in the tree.
//
// Unlink Get/Put operations, this operation does not perform any splay operations.
func (t *Tree[K, V]) Keys() []K {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	keys := make([]K, 0, t.size)
	var inorder func(n *Node[K, V])
	inorder = func(n *Node[K, V]) {
		if n == nil {
			return
		}
		inorder(n.Children[0])
		keys = append(keys, n.Key)
		inorder(n.Children[1])
	}
	inorder(t.root)
	return keys
}

// Values returns a slice of all values in the tree.
//
// Unlike Get/Put operations, this operation does not perform any splay operations.
func (t *Tree[K, V]) Values() []V {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	values := make([]V, 0, t.size)
	var inorder func(n *Node[K, V])
	inorder = func(n *Node[K, V]) {
		if n == nil {
			return
		}
		inorder(n.Children[0])
		values = append(values, n.Value)
		inorder(n.Children[1])
	}
	inorder(t.root)
	return values
}

// Min returns the minimum key in the tree and its associated value.
// If the tree is empty, it returns the zero values for K and V, and false.
// Otherwise returns the key-value pair and true.
//
// This operation modifies the tree structure due to splaying if found.
func (t *Tree[K, V]) Min() (K, V, bool) {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	if t.root == nil {
		var k K
		var v V
		return k, v, false
	}

	node := t.root
	for node.Children[0] != nil {
		node = node.Children[0]
	}
	t.splay(node)
	return node.Key, node.Value, true
}

// Max returns the maximum key in the tree and its associated value.
// If the tree is empty, it returns the zero values for K and V, and false.
// Otherwise returns the key-value pair and true.
//
// This operation modifies the tree structure due to splaying if found.
func (t *Tree[K, V]) Max() (K, V, bool) {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	if t.root == nil {
		var k K
		var v V
		return k, v, false
	}

	node := t.root
	for node.Children[1] != nil {
		node = node.Children[1]
	}
	t.splay(node)
	return node.Key, node.Value, true
}

// Floor returns the largest key in the tree that is less than or equal to the given key.
// If no such key exists, returns zero values and false.
// If the exact key exists, returns that key-value pair and true.
//
// This operation modifies the tree structure due to splaying if found.
func (t *Tree[K, V]) Floor(key K) (K, V, bool) {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	if t.root == nil {
		var k K
		var v V
		return k, v, false
	}

	var lastLE *Node[K, V] // last less or equal node
	node := t.root

	for node != nil {
		switch res := t.cmp(key, node.Key); {
		case res == 0:
			t.splay(node)
			return node.Key, node.Value, true
		case res < 0:
			node = node.Children[0]
		default:
			lastLE = node
			node = node.Children[1]
		}
	}

	if lastLE != nil {
		t.splay(lastLE)
		return lastLE.Key, lastLE.Value, true
	}

	var k K
	var v V
	return k, v, false
}

// Ceiling returns the smallest key in the tree that is greater than or equal to the given key.
// If no such key exists, returns zero values and false.
// If the exact key exists, returns that key-value pair and true.
//
// This operation modifies the tree structure due to splaying if found.
func (t *Tree[K, V]) Ceiling(key K) (K, V, bool) {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	if t.root == nil {
		var k K
		var v V
		return k, v, false
	}

	var firstGE *Node[K, V] // first greater or equal node
	node := t.root

	for node != nil {
		switch res := t.cmp(key, node.Key); {
		case res == 0:
			t.splay(node)
			return node.Key, node.Value, true
		case res < 0:
			firstGE = node
			node = node.Children[0]
		default:
			node = node.Children[1]
		}
	}

	if firstGE != nil {
		t.splay(firstGE)
		return firstGE.Key, firstGE.Value, true
	}

	var k K
	var v V
	return k, v, false
}

// PreOrder traverses the tree in pre-order (root-left-right) fashion,
// calling the provided function for each node.
// If the function returns false, the traversal is stopped.
//
// Unlike Get/Put operations, PreOrder does not perform any splay operations.
func (t *Tree[K, V]) PreOrder(fn func(key K, value V) bool) {
	if fn == nil {
		return
	}
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}
	if t.root == nil {
		return
	}

	var preorder func(n *Node[K, V]) bool
	preorder = func(n *Node[K, V]) bool {
		if n == nil {
			return true
		}

		if !fn(n.Key, n.Value) {
			return false
		}

		if !preorder(n.Children[0]) {
			return false
		}

		return preorder(n.Children[1])
	}

	preorder(t.root)
}

// InOrder traverses the tree in-order (left-root-right) fashion,
// calling the provided function for each node.
// If the function returns false, the traversal is stopped.
// The traversal visits nodes in ascending order of keys.
//
// Unlike Get/Put operations, InOrder does not perform any splay operations.
func (t *Tree[K, V]) InOrder(fn func(key K, value V) bool) {
	if fn == nil {
		return
	}
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}
	if t.root == nil {
		return
	}

	var inorder func(n *Node[K, V]) bool
	inorder = func(n *Node[K, V]) bool {
		if n == nil {
			return true
		}

		if !inorder(n.Children[0]) {
			return false
		}

		if !fn(n.Key, n.Value) {
			return false
		}

		return inorder(n.Children[1])
	}
	inorder(t.root)
}

// PostOrder traverses the tree in post-order (left-right-root) fashion,
// calling the provided function for each node.
// If the function returns false, the traversal is stopped.
//
// Unlike Get/Put operations, PostOrder does not perform any splay operations.
func (t *Tree[K, V]) PostOrder(fn func(key K, value V) bool) {
	if fn == nil {
		return
	}
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}
	if t.root == nil {
		return
	}

	var postorder func(n *Node[K, V]) bool
	postorder = func(n *Node[K, V]) bool {
		if n == nil {
			return true
		}

		if !postorder(n.Children[0]) {
			return false
		}

		if !postorder(n.Children[1]) {
			return false
		}

		return fn(n.Key, n.Value)
	}

	postorder(t.root)
}

// LevelOrder traverses the tree in level-order (breadth-first) fashion,
// calling the provided function for each node.
// If the function returns false, the traversal is stopped.
//
// Unlike Get/Put operations, LevelOrder does not perform any splay operations.
func (t *Tree[K, V]) LevelOrder(fn func(key K, value V) bool) {
	if fn == nil {
		return
	}
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}
	if t.root == nil {
		return
	}

	queue := []*Node[K, V]{t.root}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if !fn(node.Key, node.Value) {
			return
		}
		if node.Children[0] != nil {
			queue = append(queue, node.Children[0])
		}
		if node.Children[1] != nil {
			queue = append(queue, node.Children[1])
		}
	}
}

// String returns a string representation of the tree.
func (t *Tree[K, V]) String() string {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	if t.root == nil {
		return "SplayTree (empty)"
	}
	// str := "SplayTree\n"
	// output(t.root, "", true, &str, t.nodeFormat)
	// return str
	var sb strings.Builder
	sb.WriteString("SplayTree\n")
	output2(t.root, "", true, &sb, t.nodeFormat)
	return sb.String()
}

func output2[K comparable, V any](node *Node[K, V], prefix string, isTail bool, sb *strings.Builder, format string) {
	if node.Children[1] != nil {
		newPrefix := prefix
		if isTail {
			newPrefix += vertical
		} else {
			newPrefix += space
		}
		output2(node.Children[1], newPrefix, false, sb, format)
	}

	sb.WriteString(prefix)
	if isTail {
		sb.WriteString(tailMark)
	} else {
		sb.WriteString(headMark)
	}

	if len(format) > 0 {
		fmt.Fprintf(sb, format, node.Key, node.Value)
	} else {
		fmt.Fprintf(sb, "%v\n", node.String())
	}

	if node.Children[0] != nil {
		newPrefix := prefix
		if isTail {
			newPrefix += space
		} else {
			newPrefix += vertical
		}
		output2(node.Children[0], newPrefix, true, sb, format)
	}
}

func (t *Tree[K, V]) splay(x *Node[K, V]) {
	for x.Parent != nil {
		p := x.Parent
		g := p.Parent

		if g == nil {
			// zig
			if p.Children[0] == x {
				t.root = rotateZigRight(p)
			} else {
				t.root = rotateZigLeft(p)
			}
		} else {
			// check zig-zig
			if g.Children[0] == p && p.Children[0] == x {
				g = rotateZigZigRight(g)
			} else if g.Children[1] == p && p.Children[1] == x {
				g = rotateZigZigLeft(g)
			} else if g.Children[0] == p && p.Children[1] == x {
				g = rotateZigZagLeftRight(g)
			} else {
				g = rotateZigZagRightLeft(g)
			}

			if g.Parent == nil {
				t.root = g
			}
		}
	}
}
