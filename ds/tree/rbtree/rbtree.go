package rbtree

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/hydroan/gst/ds/types"
)

// Tree represents a generic red-black tree.
// It support keys of any comparable type and value of any type.
// The tree use a custom comparison function to matain order.
type Tree[K comparable, V any] struct {
	root *Node[K, V]
	size int
	cmp  func(K, K) int

	safe          bool
	mu            types.Locker
	color         bool
	nodeFormatter func(K, V) string
}

// New creates and returns a red-black tree.
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

// NewOrderedKeys creates and returns a red-black tree.
// It use the cmp.Compare[K] as the default comparison function.
// This is suitable for types that implement the cmp.Ordered interface,
// such as int, float64 and string
func NewOrderedKeys[K cmp.Ordered, V any](ops ...Option[K, V]) (*Tree[K, V], error) {
	return New(cmp.Compare[K], ops...)
}

// NewFromSlice creates and returns a red-black tree from a given slice.
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

// NewFromMap creates and returns a red-black tree from a given map.
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

// NewFromOrderedMap creates and returns a red-black tree from a given map.
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

// Put inserts a key-value pair into the tree.
// if the key already exists, its value will be updated.
func (t *Tree[K, V]) Put(key K, val V) {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	t.put(key, val)
}

func (t *Tree[K, V]) put(key K, val V) {
	if t.root == nil {
		t.root = &Node[K, V]{Key: key, Value: val, color: black}
		t.size++
		return
	}

	curr := t.root
	var newNode *Node[K, V]
	for {
		res := t.cmp(curr.Key, key)
		switch {
		case res == 0:
			curr.Value = val
			return
		case res > 0:
			if curr.Left == nil {
				newNode = &Node[K, V]{Key: key, Value: val, Parent: curr, color: red}
				curr.Left = newNode
				t.size++
				t.insertCase1(newNode)
				return
			}
			curr = curr.Left
		case res < 0:
			if curr.Right == nil {
				newNode = &Node[K, V]{Key: key, Value: val, Parent: curr, color: red}
				curr.Right = newNode
				t.size++
				t.insertCase1(newNode)
				return
			}
			curr = curr.Right
		}
	}
}

// Get returns the value associated with the given key.
func (t *Tree[K, V]) Get(key K) (value V, found bool) {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	n := t.lookup(key)
	if n != nil {
		return n.Value, true
	}
	return value, false
}

// Delete removes the node with the given key from the tree.
func (t *Tree[K, V]) Delete(key K) (V, bool) {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	var child *Node[K, V]
	n := t.lookup(key)
	if n == nil {
		var v V
		return v, false
	}
	deletedVal := n.Value
	if n.Left != nil && n.Right != nil {
		pred := n.Left.maximum()
		n.Key = pred.Key
		n.Value = pred.Value
		n = pred
	}
	if n.Left == nil || n.Right == nil {
		if n.Right == nil {
			child = n.Left
		} else {
			child = n.Right
		}
		if n.color == black {
			n.color = colorOf(child)
			t.deleteCase1(n)
		}
		t.replace(n, child)
		if n.Parent == nil && child != nil {
			child.color = black
		}
	}
	t.size--
	return deletedVal, true
}

// IsEmpty reports whether the tree is empty.
func (t *Tree[K, V]) IsEmpty() bool {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	return t.size == 0
}

// Size returns the number of nodes in the tree.
func (t *Tree[K, V]) Size() int {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	return t.size
}

// Clear clears the tree.
func (t *Tree[K, V]) Clear() {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	t.root = nil
	t.size = 0
}

// Keys returns a slice containing all keys in sorted order.
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
		inorder(n.Left)
		keys = append(keys, n.Key)
		inorder(n.Right)
	}
	inorder(t.root)
	return keys
}

// Values returns a slice containing all values in sorted order.
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
		inorder(n.Left)
		values = append(values, n.Value)
		inorder(n.Right)
	}
	inorder(t.root)
	return values
}

// Min returns the minimum node in the tree.
// If the tree is empty, it returns the nil.
func (t *Tree[K, V]) Min() (K, V, bool) {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	if t.root == nil {
		var k K
		var v V
		return k, v, false
	}
	curr := t.root
	for curr != nil && curr.Left != nil {
		curr = curr.Left
	}
	return curr.Key, curr.Value, true
}

// Max returns the maximum node in the tree.
// If the tree is empty, it returns the nil.
func (t *Tree[K, V]) Max() (K, V, bool) {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	if t.root == nil {
		var k K
		var v V
		return k, v, false
	}
	curr := t.root
	for curr != nil && curr.Right != nil {
		curr = curr.Right
	}
	return curr.Key, curr.Value, true
}

// Floor returns the largest node with a key less than or equal to the given key.
// If such a node exists, it is returned along with true; otherwise, nil and false are returned.
func (t *Tree[K, V]) Floor(key K) (K, V, bool) {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	if t.root == nil {
		var k K
		var v V
		return k, v, false
	}
	n := t.root
	var floor *Node[K, V]
	for n != nil {
		res := t.cmp(n.Key, key)
		switch {
		case res == 0:
			return n.Key, n.Value, true
		case res > 0:
			n = n.Left
		case res < 0:
			floor = n
			n = n.Right
		}
	}

	if floor != nil {
		return floor.Key, floor.Value, true
	}

	var k K
	var v V
	return k, v, false
}

// Ceiling returns the smallest node with a key greater than or equal to the given key.
// If such a node exists, it is returned along with true; otherwise, nil and false are returned.
func (t *Tree[K, V]) Ceiling(key K) (K, V, bool) {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	if t.root == nil {
		var k K
		var v V
		return k, v, false
	}
	n := t.root
	var ceiling *Node[K, V]
	for n != nil {
		res := t.cmp(n.Key, key)
		switch {
		case res == 0:
			return n.Key, n.Value, true
		case res > 0:
			ceiling = n
			n = n.Left
		case res < 0:
			n = n.Right
		}
	}

	if ceiling != nil {
		return ceiling.Key, ceiling.Value, true
	}

	var k K
	var v V
	return k, v, false
}

// // PreorderChan returns a channel that emits tree nodes in preorder traversal order.
// // The traversal starts from the root and follows: node → left subtree → right subtree.
// func (t *Tree[K, V]) PreorderChan() <-chan *Node[K, V] {
// 	if t.safe {
// 		t.mu.RLock()
// 		defer t.mu.RUnlock()
// 	}
//
// 	ch := make(chan *Node[K, V])
// 	go func() {
// 		defer close(ch)
// 		if t.root == nil {
// 			return
// 		}
// 		var traverse func(*Node[K, V])
// 		traverse = func(n *Node[K, V]) {
// 			if n == nil {
// 				return
// 			}
// 			ch <- n
// 			traverse(n.Left)
// 			traverse(n.Right)
// 		}
// 		traverse(t.root)
// 	}()
// 	return ch
// }

// PreOrder call function "fn" on each node in preorder traversal order.
// The traversal starts from the root and follows: node → left subtree → right subtree
func (t *Tree[K, V]) PreOrder(fn func(K, V) bool) {
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
		if !preorder(n.Left) {
			return false
		}
		return preorder(n.Right)
	}
	preorder(t.root)
}

// // InorderChan returns a channel that emits tree nodes in inorder traversal order.
// // The traversal follows: left subtree → node → right subtree,
// // producing nodes in sorted order for a binary search tree.
// func (t *Tree[K, V]) InorderChan() <-chan *Node[K, V] {
// 	if t.safe {
// 		t.mu.RLock()
// 		defer t.mu.RUnlock()
// 	}
//
// 	ch := make(chan *Node[K, V])
// 	go func() {
// 		defer close(ch)
// 		if t.root == nil {
// 			return
// 		}
// 		var traverse func(*Node[K, V])
// 		traverse = func(n *Node[K, V]) {
// 			if n == nil {
// 				return
// 			}
// 			traverse(n.Left)
// 			ch <- n
// 			traverse(n.Right)
// 		}
// 		traverse(t.root)
// 	}()
// 	return ch
// }

// InOrder call function "fn" on each node in inorder traversal order.
// The traversal follows: left subtree → node → right subtree,
func (t *Tree[K, V]) InOrder(fn func(K, V) bool) {
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
		if !inorder(n.Left) {
			return false
		}
		if !fn(n.Key, n.Value) {
			return false
		}
		return inorder(n.Right)
	}
	inorder(t.root)
}

// // PostorderChan returns a channel that emits tree nodes in postorder traversal order.
// // The traversal follows: left subtree → right subtree → node,
// func (t *Tree[K, V]) PostorderChan() <-chan *Node[K, V] {
// 	if t.safe {
// 		t.mu.RLock()
// 		defer t.mu.RUnlock()
// 	}
//
// 	ch := make(chan *Node[K, V])
// 	go func() {
// 		defer close(ch)
// 		if t.root == nil {
// 			return
// 		}
// 		var traverse func(*Node[K, V])
// 		traverse = func(n *Node[K, V]) {
// 			if n == nil {
// 				return
// 			}
// 			traverse(n.Left)
// 			traverse(n.Right)
// 			ch <- n
// 		}
// 		traverse(t.root)
// 	}()
// 	return ch
// }

// PostOrder call function "fn" on each node in postorder traversal order.
// The traversal follows: left subtree → right subtree → node
func (t *Tree[K, V]) PostOrder(fn func(K, V) bool) {
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
		if !postorder(n.Left) {
			return false
		}
		if !postorder(n.Right) {
			return false
		}
		return fn(n.Key, n.Value)
	}
	postorder(t.root)
}

// // LevelOrderChan returns a channel that emits tree nodes in level-order traversal order.
// // The traversal visits nodes level by level from left to right, starting from the root.
// func (t *Tree[K, V]) LevelOrderChan() <-chan *Node[K, V] {
// 	if t.safe {
// 		t.mu.RLock()
// 		defer t.mu.RUnlock()
// 	}
//
// 	ch := make(chan *Node[K, V])
// 	go func() {
// 		defer close(ch)
// 		if t.root == nil {
// 			return
// 		}
// 		queue := []*Node[K, V]{t.root}
// 		for len(queue) > 0 {
// 			n := queue[0]
// 			queue = queue[1:]
// 			ch <- n
// 			if n.Left != nil {
// 				queue = append(queue, n.Left)
// 			}
// 			if n.Right != nil {
// 				queue = append(queue, n.Right)
// 			}
// 		}
// 	}()
// 	return ch
// }

// LevelOrder call function "fn" on each node in levelorder traversal order
func (t *Tree[K, V]) LevelOrder(fn func(K, V) bool) {
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
		if node.Left != nil {
			queue = append(queue, node.Left)
		}
		if node.Right != nil {
			queue = append(queue, node.Right)
		}
	}
}

func (t *Tree[K, V]) BlackCount() int {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	return blackCount(t.root)
}

func blackCount[K comparable, V any](n *Node[K, V]) int {
	if n == nil {
		return 0
	}
	count := blackCount(n.Left) + blackCount(n.Right)
	if colorOf(n) == black {
		count++
	}
	return count
}

func (t *Tree[K, V]) RedCount() int {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	return redCount(t.root)
}

func redCount[K comparable, V any](n *Node[K, V]) int {
	if n == nil {
		return 0
	}
	count := redCount(n.Left) + redCount(n.Right)
	if colorOf(n) == red {
		count++
	}
	return count
}

func (t *Tree[K, V]) LeafCount() int {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	return leafCount(t.root)
}

func leafCount[K comparable, V any](n *Node[K, V]) int {
	if n == nil {
		return 0
	}
	if n.Left == nil && n.Right == nil {
		return 1
	}
	return leafCount(n.Left) + leafCount(n.Right)
}

func (t *Tree[K, V]) MaxDepth() int {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	if t.root == nil {
		return 0
	}
	return maxDepth(t.root)
}

func maxDepth[K comparable, V any](n *Node[K, V]) int {
	if n == nil {
		return 0
	}
	return max(maxDepth(n.Left), maxDepth(n.Right)) + 1
}

func (t *Tree[K, V]) MinDepth() int {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	if t.root == nil {
		return 0
	}
	return minDepth(t.root)
}

func minDepth[K comparable, V any](n *Node[K, V]) int {
	if n == nil {
		return 0
	}
	if n.Left == nil && n.Right == nil {
		return 1
	}
	if n.Left == nil {
		return minDepth(n.Right) + 1
	}
	if n.Right == nil {
		return minDepth(n.Left) + 1
	}
	return min(minDepth(n.Left), minDepth(n.Right)) + 1
}

// String returns a visual representation of the Red-Black Tree.
// If t.color is true, nodes will be displayed in terminal colors.
func (t *Tree[K, V]) String() string {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}
	if t.root == nil {
		return "RedBlackTree (empty)"
	}

	var sb strings.Builder
	sb.WriteString("RedBlackTree\n")
	nodeFormatter := t.nodeFormatter
	if nodeFormatter == nil {
		nodeFormatter = func(k K, v V) string { return fmt.Sprintf("%v", k) }
	}
	t.output(t.root, "", true, &sb, nodeFormatter)
	return sb.String()
}

// output recursively builds the tree structure as a string.
func (t *Tree[K, V]) output(node *Node[K, V], prefix string, isTail bool, sb *strings.Builder, nodeFormatter func(K, V) string) {
	if node == nil {
		return
	}

	// right node
	if node.Right != nil {
		var newPrefix string
		if isTail {
			newPrefix = prefix + "│   "
		} else {
			newPrefix = prefix + "    "
		}
		t.output(node.Right, newPrefix, false, sb, nodeFormatter)
	}

	// current node
	sb.WriteString(prefix)
	if isTail {
		sb.WriteString("╰── ")
	} else {
		sb.WriteString("╭── ")
	}

	colorCode := ""
	if t.color {
		if colorOf(node) == red {
			// colorCode = RedTxt
			colorCode = RedBg
		} else {
			// colorCode = BlkTxt
			colorCode = BlackBg
		}
	}
	fmt.Fprintf(sb, "%s%v(%s)%s\n", colorCode, nodeFormatter(node.Key, node.Value), node.color.symbol(), Reset)

	// left node
	if node.Left != nil {
		var newPrefix string
		if isTail {
			newPrefix = prefix + "    "
		} else {
			newPrefix = prefix + "│   "
		}
		t.output(node.Left, newPrefix, true, sb, nodeFormatter)
	}
}

func (t *Tree[K, V]) rotateLeft(n *Node[K, V]) {
	r := n.Right
	t.replace(n, r)
	n.Right = r.Left
	if r.Left != nil {
		r.Left.Parent = n
	}
	r.Left = n
	n.Parent = r
}

func (t *Tree[K, V]) rotateRight(n *Node[K, V]) {
	l := n.Left
	t.replace(n, l)
	n.Left = l.Right
	if l.Right != nil {
		l.Right.Parent = n
	}
	l.Right = n
	n.Parent = l
}

func (t *Tree[K, V]) insertCase1(n *Node[K, V]) {
	if n.Parent == nil {
		n.color = black
	} else {
		t.insertCase2(n)
	}
}

func (t *Tree[K, V]) insertCase2(n *Node[K, V]) {
	if colorOf(n.Parent) == black {
		return
	}
	t.insertCase3(n)
}

func (t *Tree[K, V]) insertCase3(n *Node[K, V]) {
	g := n.grandparent()
	p := n.Parent
	u := n.uncle()

	if colorOf(u) == red {
		p.color = black
		u.color = black
		g.color = red
		t.insertCase1(g)
	} else {
		t.insertCase4(n)
	}
}

func (t *Tree[K, V]) insertCase4(n *Node[K, V]) {
	g := n.grandparent()
	p := n.Parent

	if n == p.Right && p == g.Left {
		t.rotateLeft(p)
		n = n.Left
	} else if n == p.Left && p == g.Right {
		t.rotateRight(p)
		n = n.Right
	}
	t.insertCase5(n)
}

func (t *Tree[K, V]) insertCase5(n *Node[K, V]) {
	g := n.grandparent()
	p := n.Parent

	p.color = black
	g.color = red
	if n == p.Left && p == g.Left {
		t.rotateRight(g)
	} else if n == p.Right && p == g.Right {
		t.rotateLeft(g)
	}
}

func (t *Tree[K, V]) deleteCase1(n *Node[K, V]) {
	if n.Parent == nil {
		return
	}
	t.deleteCase2(n)
}

func (t *Tree[K, V]) deleteCase2(n *Node[K, V]) {
	p := n.Parent
	s := n.sibling()

	if colorOf(s) == red {
		p.color = red
		s.color = black
		if n == p.Left {
			t.rotateLeft(p)
		} else {
			t.rotateRight(p)
		}
	}
	t.deleteCase3(n)
}

func (t *Tree[K, V]) deleteCase3(n *Node[K, V]) {
	p := n.Parent
	s := n.sibling()

	if colorOf(p) == black &&
		colorOf(s) == black &&
		colorOf(s.Left) == black &&
		colorOf(s.Right) == black {
		s.color = red
		t.deleteCase1(p)
	} else {
		t.deleteCase4(n)
	}
}

func (t *Tree[K, V]) deleteCase4(n *Node[K, V]) {
	p := n.Parent
	s := n.sibling()

	if colorOf(p) == red &&
		colorOf(s) == black &&
		colorOf(s.Left) == black &&
		colorOf(s.Right) == black {
		s.color = red
		p.color = black
	} else {
		t.deleteCase5(n)
	}
}

func (t *Tree[K, V]) deleteCase5(node *Node[K, V]) {
	p := node.Parent
	s := node.sibling()

	if node == p.Left &&
		colorOf(s) == black &&
		colorOf(s.Left) == red &&
		colorOf(s.Right) == black {
		s.color = red
		s.Left.color = black
		t.rotateRight(s)
	} else if node == p.Right &&
		colorOf(s) == black &&
		colorOf(s.Right) == red &&
		colorOf(s.Left) == black {
		s.color = red
		s.Right.color = black
		t.rotateLeft(s)
	}
	t.deleteCase6(node)
}

func (t *Tree[K, V]) deleteCase6(node *Node[K, V]) {
	p := node.Parent
	s := node.sibling()

	s.color = colorOf(p)
	p.color = black
	if node == p.Left && colorOf(s.Right) == red {
		s.Right.color = black
		t.rotateLeft(p)
	} else if colorOf(s.Left) == red {
		s.Left.color = black
		t.rotateRight(p)
	}
}

func (t *Tree[K, V]) lookup(key K) *Node[K, V] {
	n := t.root
	for n != nil {
		res := t.cmp(n.Key, key)
		switch {
		case res == 0:
			return n
		case res > 0:
			n = n.Left
		case res < 0:
			n = n.Right
		}
	}
	return nil
}

func (t *Tree[K, V]) replace(old *Node[K, V], new_ *Node[K, V]) {
	if old.Parent == nil {
		t.root = new_
	} else if old == old.Parent.Left {
		old.Parent.Left = new_
	} else {
		old.Parent.Right = new_
	}
	if new_ != nil {
		new_.Parent = old.Parent
	}
}

func colorOf[K comparable, V any](n *Node[K, V]) color {
	if n == nil {
		return black
	}
	return n.color
}
