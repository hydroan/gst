package avltree

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/hydroan/gst/ds/types"
)

// Tree represents a generic AVL tree.
// It support keys of any comparable type and value of any type.
// The tree use a custom comparison function to matain order.
type Tree[K comparable, V any] struct {
	root *Node[K, V]
	cmp  func(K, K) int
	size int

	safe          bool
	mu            types.Locker
	nodeFormatter func(K, V) string
}

// New creates and returns a AVL tree.
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

// NewOrderedKeys creates and returns a AVL tree.
// It use the cmp.Compare[K] as the default comparison function.
// This is suitable for types that implement the cmp.Ordered interface,
// such as int, float64 and string
func NewOrderedKeys[K cmp.Ordered, V any](ops ...Option[K, V]) (*Tree[K, V], error) {
	return New(cmp.Compare[K], ops...)
}

// NewFromSlice creates and returns a AVL tree from a given slice.
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

// NewFromMap creates and returns a AVL tree from a given map.
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

// NewFromOrderedMap creates and returns a AVL tree from a given map.
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

// Put insert a key-pair into the tree.
// If the key already exists, its value will be updated.
func (t *Tree[K, V]) Put(key K, val V) {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	t.put(key, val, nil, &t.root)
}

// ref: https://github.com/emirpasic/gods/blob/8323d02ee3ca1499478f9ccd7a299fb1c5005780/trees/avltree/avltree.go#L225
func (t *Tree[K, V]) put(key K, value V, p *Node[K, V], qp **Node[K, V]) bool {
	q := *qp
	if q == nil {
		t.size++
		*qp = &Node[K, V]{Key: key, Value: value, Parent: p}
		return true
	}

	res := t.cmp(key, q.Key)
	if res == 0 {
		q.Key = key
		q.Value = value
		return false
	}

	if res < 0 {
		res = -1
	} else {
		res = 1
	}
	a := (res + 1) / 2
	fix := t.put(key, value, q, &q.Children[a])
	if fix {
		return rebalancePut(res, qp)
	}
	return false
}

// rebalancePut will rebalance after "Put".
//
// references: https://github.com/emirpasic/gods/blob/8323d02ee3ca1499478f9ccd7a299fb1c5005780/trees/avltree/avltree.go#L308
func rebalancePut[K comparable, V any](c int, t **Node[K, V]) bool {
	s := *t
	if s.b == 0 {
		s.b = c
		return true
	}

	if s.b == -c {
		s.b = 0
		return false
	}

	if s.Children[(c+1)/2].b == c {
		s = singleRotate(c, s)
	} else {
		s = doubleRotate(c, s)
	}
	*t = s
	return false
}

// Get returns the value associated with the given key.
func (t *Tree[K, V]) Get(key K) (v V, found bool) {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	n, found := t.lookup(key)
	if found {
		return n.Value, true
	}
	return v, false
}

func (t *Tree[K, V]) lookup(key K) (*Node[K, V], bool) {
	n := t.root
	for n != nil {
		res := t.cmp(n.Key, key)
		switch {
		case res == 0:
			return n, true
		case res > 0:
			n = n.Children[0]
		case res < 0:
			n = n.Children[1]
		}
	}
	return nil, false
}

// Delete delete the node from the tree by key.
func (t *Tree[K, V]) Delete(key K) {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	t.delete(key, &t.root)
}

// ref: https://github.com/emirpasic/gods/blob/8323d02ee3ca1499478f9ccd7a299fb1c5005780/trees/avltree/avltree.go#L254
func (t *Tree[K, V]) delete(key K, qp **Node[K, V]) bool {
	q := *qp
	if q == nil {
		return false
	}

	res := t.cmp(key, q.Key)
	if res == 0 {
		t.size--
		if q.Children[1] == nil {
			if q.Children[0] != nil {
				q.Children[0].Parent = q.Parent
			}
			*qp = q.Children[0]
			return true
		}
		fix := removeMin(&q.Children[1], &q.Key, &q.Value)
		if fix {
			return rebalanceDelete(-1, qp)
		}
		return false
	}

	if res < 0 {
		res = -1
	} else {
		res = 1
	}
	a := (res + 1) / 2
	fix := t.delete(key, &q.Children[a])
	if fix {
		return rebalanceDelete(-res, qp)
	}
	return false
}

// removeMin will remove the minimum node.
//
// references: https://github.com/emirpasic/gods/blob/8323d02ee3ca1499478f9ccd7a299fb1c5005780/trees/avltree/avltree.go#L290
func removeMin[K comparable, V any](qp **Node[K, V], minKey *K, minVal *V) bool {
	q := *qp
	if q.Children[0] == nil {
		*minKey = q.Key
		*minVal = q.Value
		if q.Children[1] != nil {
			q.Children[1].Parent = q.Parent
		}
		*qp = q.Children[1]
		return true
	}
	fix := removeMin(&q.Children[0], minKey, minVal)
	if fix {
		return rebalanceDelete(1, qp)
	}
	return false
}

// rebalanceDelete will rebalance after "Delete".
//
// references: https://github.com/emirpasic/gods/blob/8323d02ee3ca1499478f9ccd7a299fb1c5005780/trees/avltree/avltree.go#L329
func rebalanceDelete[K comparable, V any](c int, t **Node[K, V]) bool {
	s := *t
	if s.b == 0 {
		s.b = c
		return false
	}

	if s.b == -c {
		s.b = 0
		return true
	}

	a := (c + 1) / 2
	if s.Children[a].b == 0 {
		s = rotate(c, s)
		s.b = -c
		*t = s
		return false
	}

	if s.Children[a].b == c {
		s = singleRotate(c, s)
	} else {
		s = doubleRotate(c, s)
	}
	*t = s
	return true
}

// Size returns the number of nodes in the tree.
func (t *Tree[K, V]) Size() int {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	return t.size
}

// IsEmpty reports whether the tree is empty.
func (t *Tree[K, V]) IsEmpty() bool {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	return t.Size() == 0
}

// Height returns the height of the tree.
// The height is the length of the longest path from root to leaf.
func (t *Tree[K, V]) Height() int {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	return height(t.root)
}

func height[K comparable, V any](n *Node[K, V]) int {
	if n == nil {
		return 0
	}
	lh := height(n.Children[0])
	rh := height(n.Children[1])
	if lh > rh {
		return lh + 1
	}
	return rh + 1
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
		inorder(n.Children[0])
		keys = append(keys, n.Key)
		inorder(n.Children[1])
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
func (t *Tree[K, V]) Min() (K, V, bool) {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	n := t.bottom(0)
	if n == nil {
		var k K
		var v V
		return k, v, false
	}
	return n.Key, n.Value, true
}

// Max returns the maximum key in the tree and its associated value.
// If the tree is empty, it returns the zero values for K and V, and false.
// Otherwise returns the key-value pair and true.
func (t *Tree[K, V]) Max() (K, V, bool) {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	n := t.bottom(1)
	if n == nil {
		var k K
		var v V
		return k, v, false
	}
	return n.Key, n.Value, true
}

// Floor returns the largest key in the tree that is less than or equal to the given key.
// If no such key exists, returns zero values and false.
// If the exact key exists, returns that key-value pair and true.
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

	var lastLE *Node[K, V] // last less or equal node
	node := t.root

	for node != nil {
		switch res := t.cmp(key, node.Key); {
		case res == 0:
			return node.Key, node.Value, true
		case res < 0:
			node = node.Children[0]
		default:
			lastLE = node
			node = node.Children[1]
		}
	}

	if lastLE != nil {
		return lastLE.Key, lastLE.Value, true
	}

	var k K
	var v V
	return k, v, false
}

// Ceiling returns the smallest key in the tree that is greater than or equal to the given key.
// If no such key exists, returns zero values and false.
// If the exact key exists, returns that key-value pair and true.
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

	var firstGE *Node[K, V] // first greater or equal node
	node := t.root

	for node != nil {
		switch res := t.cmp(key, node.Key); {
		case res == 0:
			return node.Key, node.Value, true
		case res < 0:
			firstGE = node
			node = node.Children[0]
		default:
			node = node.Children[1]
		}
	}

	if firstGE != nil {
		return firstGE.Key, firstGE.Value, true
	}

	var k K
	var v V
	return k, v, false
}

// // PreorderChan returns a channel that emits tree nodes in preorder traversal order.
// // The traversal starts from the root and follows: node → left subtree → right subtree
// func (t *Tree[K, V]) PreorderChan() <-chan *Node[K, V] {
// 	if t.safe {
// 		t.mu.RLock()
// 		defer t.mu.RUnlock()
// 	}
//
// 	ch := make(chan *Node[K, V])
// 	go func() {
// 		defer close(ch)
// 		var traverse func(*Node[K, V])
// 		traverse = func(n *Node[K, V]) {
// 			if n == nil {
// 				return
// 			}
// 			ch <- n
// 			traverse(n.Children[0])
// 			traverse(n.Children[1])
// 		}
// 		traverse(t.root)
// 	}()
// 	return ch
// }

// PreOrder call function "fn" on each node in preorder traversal order.
// The traversal starts from the root and follows: node → left subtree → right subtree
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

// // InorderChan returns a channel that emits tree nodes in inorder traversal order.
// // The traversal starts from the root and follows: left subtree → node → right subtree
// func (t *Tree[K, V]) InorderChan() <-chan *Node[K, V] {
// 	if t.safe {
// 		t.mu.RLock()
// 		defer t.mu.RUnlock()
// 	}
//
// 	ch := make(chan *Node[K, V])
// 	go func() {
// 		defer close(ch)
// 		var traverse func(*Node[K, V])
// 		traverse = func(n *Node[K, V]) {
// 			if n == nil {
// 				return
// 			}
// 			traverse(n.Children[0])
// 			ch <- n
// 			traverse(n.Children[1])
// 		}
// 		traverse(t.root)
// 	}()
// 	return ch
// }

// InOrder call function "fn" on each node in inorder traversal order.
// The traversal starts from the root and follows: left subtree → node → right subtree
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

// // PostorderChan returns a channel that emits tree nodes in postorder traversal order.
// // The traversal starts from the root and follows: left subtree → right subtree → node
// func (t *Tree[K, V]) PostorderChan() <-chan *Node[K, V] {
// 	if t.safe {
// 		t.mu.RLock()
// 		defer t.mu.RUnlock()
// 	}
//
// 	ch := make(chan *Node[K, V])
// 	go func() {
// 		defer close(ch)
// 		var traverse func(*Node[K, V])
// 		traverse = func(n *Node[K, V]) {
// 			if n == nil {
// 				return
// 			}
// 			traverse(n.Children[0])
// 			traverse(n.Children[1])
// 			ch <- n
// 		}
// 		traverse(t.root)
// 	}()
// 	return ch
// }

// PostOrder call function "fn" on each node in postorder traversal order.
// The traversal starts from the root and follows: left subtree → right subtree → node
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

// // LevelOrderChan returns a channel that emits tree nodes in levelorder traversal order.
// func (t *Tree[K, V]) LevelOrderChan() <-chan *Node[K, V] {
// 	if t.safe {
// 		t.mu.RLock()
// 		defer t.mu.RUnlock()
// 	}
//
// 	ch := make(chan *Node[K, V])
// 	go func() {
// 		defer close(ch)
//
// 		if t.root == nil {
// 			return
// 		}
// 		queue := []*Node[K, V]{t.root}
// 		for len(queue) > 0 {
// 			node := queue[0]
// 			queue = queue[1:]
//
// 			ch <- node
// 			if node.Children[0] != nil {
// 				queue = append(queue, node.Children[0])
// 			}
// 			if node.Children[1] != nil {
// 				queue = append(queue, node.Children[1])
// 			}
// 		}
// 	}()
// 	return ch
// }

// LevelOrder call function "fn" on each node in levelorder traversal order
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
		return "AVLTree (empty)"
	}
	var sb strings.Builder
	sb.WriteString("AVLTree\n")
	nodeFormatter := t.nodeFormatter
	if nodeFormatter == nil {
		nodeFormatter = func(k K, v V) string { return fmt.Sprintf("%v", k) }
	}
	t.output(t.root, "", true, &sb, nodeFormatter)
	return sb.String()
}

func (t *Tree[K, V]) output(node *Node[K, V], prefix string, isTail bool, sb *strings.Builder, nodeFormatter func(K, V) string) {
	if node.Children[1] != nil {
		newPrefix := prefix
		if isTail {
			newPrefix += "│   "
		} else {
			newPrefix += "    "
		}
		t.output(node.Children[1], newPrefix, false, sb, nodeFormatter)
	}

	sb.WriteString(prefix)
	if isTail {
		sb.WriteString("╰── ")
	} else {
		sb.WriteString("╭── ")
	}

	sb.WriteString(nodeFormatter(node.Key, node.Value))
	sb.WriteString("\n")

	if node.Children[0] != nil {
		newPrefix := prefix
		if isTail {
			newPrefix += "    "
		} else {
			newPrefix += "│   "
		}
		t.output(node.Children[0], newPrefix, true, sb, nodeFormatter)
	}
}

func (t *Tree[K, V]) bottom(pos int) *Node[K, V] {
	if t.root == nil {
		return nil
	}
	n := t.root
	for n.Children[pos] != nil {
		n = n.Children[pos]
	}
	return n
}

// singleRotate for LL or RR.
//
// references: https://github.com/emirpasic/gods/blob/8323d02ee3ca1499478f9ccd7a299fb1c5005780/trees/avltree/avltree.go#L358
func singleRotate[K comparable, V any](c int, s *Node[K, V]) *Node[K, V] {
	s.b = 0
	s = rotate(c, s)
	s.b = 0
	return s
}

// doubleRotate for LR or RL.
//
// references: https://github.com/emirpasic/gods/blob/8323d02ee3ca1499478f9ccd7a299fb1c5005780/trees/avltree/avltree.go#L365
func doubleRotate[K comparable, V any](c int, s *Node[K, V]) *Node[K, V] {
	a := (c + 1) / 2
	r := s.Children[a]
	s.Children[a] = rotate(-c, s.Children[a])
	p := rotate(c, s)

	switch p.b {
	default:
		s.b = 0
		r.b = 0
	case c:
		s.b = -c
		r.b = 0
	case -c:
		s.b = 0
		r.b = c
	}

	p.b = 0
	return p
}

// rotate does left rotate or right rotate.
//
//	c == -1 Left Rotate
//	c == 1 Right Rotate
//
// Children[a], Children[a^1]
//
//	a = (c + 1) / 2
//		c = -1 -> a = 0: Left Rotate
//		c = 1  -> a = 1: Right Rotate
//	a^1
//		0^1 = 1 -> Right Node
//		1^1 = 0 -> Left Node
//
// references: https://github.com/emirpasic/gods/blob/8323d02ee3ca1499478f9ccd7a299fb1c5005780/trees/avltree/avltree.go#L387
func rotate[K comparable, V any](c int, s *Node[K, V]) *Node[K, V] {
	a := (c + 1) / 2
	r := s.Children[a]
	s.Children[a] = r.Children[a^1]
	if s.Children[a] != nil {
		s.Children[a].Parent = s
	}
	r.Children[a^1] = s
	r.Parent = s.Parent
	s.Parent = r
	return r
}
