package trie

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/hydroan/gst/ds/types"
)

// Node represents a node in the trie.
// K is the key type(typically string or rune).
// V is the value type associated with the key.
type Node[K comparable, V any] struct {
	children map[K]*Node[K, V]
	value    V    // value is the value associated with the key
	hasValue bool // hasValue is true if the node has a value associated with it
	count    int  // count is the number of keys that start with this node.
}

func (n *Node[K, V]) Value() V                    { return n.value }
func (n *Node[K, V]) Children() map[K]*Node[K, V] { return n.children }

func (n *Node[K, V]) clone() *Node[K, V] {
	if n == nil {
		return nil
	}
	newN := &Node[K, V]{
		value:    n.value,
		hasValue: n.hasValue,
		count:    n.count,
		children: make(map[K]*Node[K, V], len(n.children)),
	}
	for k, child := range n.children {
		newN.children[k] = child.clone()
	}
	return newN
}

type KeysValue[K comparable, V any] struct {
	Keys  []K `json:"keys"`
	Value V   `json:"value"`
}

type Trie[K comparable, V any] struct {
	root *Node[K, V]
	size int

	safe bool
	mu   types.Locker

	nodeFormatter func(V, int, bool) string
	keyFormatter  func(K, V, int, bool) string
}

func New[K comparable, V any](ops ...Option[K, V]) (*Trie[K, V], error) {
	t := &Trie[K, V]{mu: types.FakeLocker{}}
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

func (t *Trie[K, V]) Root() *Node[K, V] { return t.root }

// Put inserts or updates a keys-value pair into the trie.
// Returns true if the key was inserted,
// false if the key already exists and value wa updated.
func (t *Trie[K, V]) Put(keys []K, val V) bool {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	return t.put(keys, val)
}

func (t *Trie[K, V]) put(keys []K, val V) bool {
	if t.root == nil {
		t.root = &Node[K, V]{children: make(map[K]*Node[K, V])}
	}
	curr := t.root
	for _, k := range keys {
		child, exists := curr.children[k]
		if !exists {
			child = &Node[K, V]{children: make(map[K]*Node[K, V])}
			curr.children[k] = child
		}
		curr = child
	}

	isNewKey := !curr.hasValue
	curr.value = val
	curr.hasValue = true
	if isNewKey {
		t.size++
		curr = t.root
		curr.count++
		for _, k := range keys {
			curr = curr.children[k]
			curr.count++
		}
	}

	return isNewKey
}

// Get retrives the value associated with the given keys.
// Returns the value and true if found, zero value and false if not foud.
func (t *Trie[K, V]) Get(keys []K) (v V, ok bool) {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}
	if t.root == nil {
		return v, false
	}

	curr := t.root
	for _, k := range keys {
		child, exists := curr.children[k]
		if !exists {
			return v, false
		}
		curr = child
	}
	return curr.value, curr.hasValue
}

// Delete removes the key and its associated value from the trie.
// Returns the deleted value and true if it exists, zero value and false otherwise.
func (t *Trie[K, V]) Delete(key []K) (v V, ok bool) {
	if len(key) == 0 {
		return v, false
	}
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}
	if t.root == nil {
		return v, false
	}

	curr := t.root
	path := make([]*Node[K, V], 0, len(key)+1)
	keys := make([]K, 0, len(key))
	path = append(path, curr)
	for _, k := range key {
		child, exists := curr.children[k]
		if !exists {
			return v, false
		}
		curr = child
		path = append(path, curr)
		keys = append(keys, k)
	}
	if !curr.hasValue {
		return v, false
	}
	v = curr.value
	curr.hasValue = false
	var zero V
	curr.value = zero
	t.size--

	// update node's count from leaf to root
	for i, node := range slices.Backward(path) {
		node.count--
		// if not root node and count is 0, remove the node
		if i > 0 && node.count <= 0 {
			parent := path[i-1]
			delete(parent.children, keys[i-1])
		}
	}
	if len(t.root.children) == 0 {
		t.root = nil
	}
	return v, true
}

// DeletePrefix removes all keys-value pairs that start with the given prefix.
// Returns the number of keys-value pairs removed.
func (t *Trie[K, V]) DeletePrefix(prefix []K) int {
	if len(prefix) == 0 {
		return 0
	}
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}
	if t.root == nil {
		return 0
	}

	curr := t.root
	path := make([]*Node[K, V], 0, len(prefix)+1)
	keys := make([]K, 0, len(prefix))
	path = append(path, curr)

	// Traverse to the node where prefix ends
	for _, k := range prefix {
		child, exists := curr.children[k]
		if !exists {
			return 0
		}
		curr = child
		path = append(path, curr)
		keys = append(keys, k)
	}

	// Count nodes to be deleted
	deleteCount := 0
	var fn func(*Node[K, V])
	fn = func(n *Node[K, V]) {
		if n == nil {
			return
		}
		if n.hasValue {
			deleteCount++
		}
		for _, child := range n.children {
			fn(child)
		}
		// Release memory
		n.children = nil
		n.count = 0
	}
	fn(curr)

	// Remove the prefix node from its parent
	if len(path) > 1 {
		parent := path[len(path)-2]
		delete(parent.children, keys[len(keys)-1])
	}

	// Update counts from bottom to top
	for i, node := range slices.Backward(path) {
		node.count -= deleteCount
		if i > 0 && node.count == 0 {
			parent := path[i-1]
			delete(parent.children, keys[i-1])
		}
	}

	// If root has no children, set it to nil
	if len(t.root.children) == 0 {
		t.root = nil
	}

	t.size -= deleteCount
	return deleteCount
}

// IsEmpty reports whether the trie is empty.
func (t *Trie[K, V]) IsEmpty() bool {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	return t.size == 0
}

// Size returns the number of keys-value pairs in the trie.
func (t *Trie[K, V]) Size() int {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	return t.size
}

// Clear removes all keys-value pairs from the trie.
func (t *Trie[K, V]) Clear() {
	if t.safe {
		t.mu.Lock()
		defer t.mu.Unlock()
	}

	t.root = nil
	t.size = 0
}

// Clone returns a deep copy of the trie.
func (t *Trie[K, V]) Clone() *Trie[K, V] {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	clone, _ := New(t.options()...)
	if t.root == nil {
		return clone
	}

	// path := make([]K, 0, t.size)
	// t._range(t.root, path, func(k []K, v V) bool {
	// 	clone.Put(k, v)
	// 	return true
	// })
	clone.root = t.root.clone()
	clone.size = t.size
	return clone
}

func (t *Trie[K, V]) options() []Option[K, V] {
	ops := make([]Option[K, V], 0)
	if t.safe {
		ops = append(ops, WithSafe[K, V]())
	}
	if t.nodeFormatter != nil {
		ops = append(ops, WithNodeFormatter[K, V](t.nodeFormatter))
	}
	if t.keyFormatter != nil {
		ops = append(ops, WithKeyFormatter(t.keyFormatter))
	}
	return ops
}

// Keys returns all keys in the trie.
// Returns empty slice(not nil) if the trie is empty.
// The order of keys is not guaranteed.
func (t *Trie[K, V]) Keys() [][]K {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}
	keys := make([][]K, 0, t.size)
	if t.root == nil {
		return keys
	}

	var path []K
	var fn func(*Node[K, V], []K)
	fn = func(n *Node[K, V], path []K) {
		if n.hasValue {
			key := make([]K, len(path))
			copy(key, path)
			keys = append(keys, key)
		}
		for k, child := range n.children {
			fn(child, append(path, k))
		}
	}
	fn(t.root, path)

	return keys
}

// Values returns all values in the trie.
// Returns empty slice(not nil) if the trie is empty.
// The order of values is not guaranteed.
func (t *Trie[K, V]) Values() []V {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}
	values := make([]V, 0, t.size)
	if t.root == nil {
		return values
	}

	var fn func(*Node[K, V])
	fn = func(n *Node[K, V]) {
		if n.hasValue {
			values = append(values, n.value)
		}
		for _, child := range n.children {
			fn(child)
		}
	}
	fn(t.root)

	// stack := []*Node[K, V]{t.root}
	// for len(stack) > 0 {
	// 	n := stack[len(stack)-1]
	// 	stack = stack[:len(stack)-1]
	// 	if n.hasValue {
	// 		values = append(values, n.value)
	// 	}
	// 	for _, child := range n.children {
	// 		stack = append(stack, child)
	// 	}
	// }

	return values
}

// KeysValues returns all keys-value pairs in the trie.
// Returns empty slice(not nil) if the trie is empty
// The order of key-value pairs is not guaranteed.
func (t *Trie[K, V]) KeysValues() []KeysValue[K, V] {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	return t.keysValues()
}

func (t *Trie[K, V]) keysValues() []KeysValue[K, V] {
	keysValues := make([]KeysValue[K, V], 0, t.size)
	if t.root == nil {
		return keysValues
	}

	var path []K
	var fn func(*Node[K, V], []K)
	fn = func(n *Node[K, V], path []K) {
		if n.hasValue {
			keys := make([]K, len(path))
			copy(keys, path)
			keysValues = append(keysValues, KeysValue[K, V]{
				Keys:  keys,
				Value: n.value,
			})
		}
		for k, child := range n.children {
			fn(child, append(path, k))
		}
	}
	fn(t.root, path)

	return keysValues
}

// PrefixKeys returns all keys that start with the given prefix.
// It will returns empty slice(not nil) if the trie is empty
// or no keys start with the given prefix.
func (t *Trie[K, V]) PrefixKeys(prefix []K) [][]K {
	if len(prefix) == 0 {
		return nil
	}
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}
	keys := make([][]K, 0)
	if t.root == nil {
		return keys
	}

	curr := t.root
	for _, k := range prefix {
		child, exists := curr.children[k]
		if !exists {
			return keys
		}
		curr = child
	}

	var fn func(*Node[K, V], []K)
	fn = func(n *Node[K, V], path []K) {
		if n.hasValue {
			key := make([]K, len(path))
			copy(key, path)
			keys = append(keys, key)
		}
		for k, child := range n.children {
			fn(child, append(path, k))
		}
	}
	fn(curr, prefix)

	return keys
}

// PrefixValues returns all values that start with the given prefix.
// It will returns empty slice(not nil) if the trie is empty
// or no values start with the given prefix.
func (t *Trie[K, V]) PrefixValues(prefix []K) []V {
	if len(prefix) == 0 {
		return nil
	}
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}
	values := make([]V, 0)
	if t.root == nil {
		return values
	}

	curr := t.root
	for _, k := range prefix {
		child, exists := curr.children[k]
		if !exists {
			return values
		}
		curr = child
	}
	var fn func(*Node[K, V])
	fn = func(n *Node[K, V]) {
		if n.hasValue {
			values = append(values, n.value)
		}
		for _, child := range n.children {
			fn(child)
		}
	}
	fn(curr)

	return values
}

// PrefixKeysValues returns the keys and values that start with the given prefix.
// It will returns empty slice(not nil) if the trie is empty
// or no keys-value pairs start with the given prefix.
func (t *Trie[K, V]) PrefixKeysValues(prefix []K) []KeysValue[K, V] {
	if len(prefix) == 0 {
		return nil
	}
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}
	keysValues := make([]KeysValue[K, V], 0)
	if t.root == nil {
		return keysValues
	}

	curr := t.root
	for _, k := range prefix {
		child, exists := curr.children[k]
		if !exists {
			return keysValues
		}
		curr = child
	}

	var fn func(*Node[K, V], []K)
	fn = func(n *Node[K, V], path []K) {
		if n.hasValue {
			keys := make([]K, len(path))
			copy(keys, path)
			keysValues = append(keysValues, KeysValue[K, V]{
				Keys:  keys,
				Value: n.value,
			})
		}
		for k, child := range n.children {
			fn(child, append(path, k))
		}
	}
	fn(curr, prefix)

	return keysValues
}

// PrefixCount returns the number of keys that start with the given prefix.
func (t *Trie[K, V]) PrefixCount(prefix []K) int {
	if len(prefix) == 0 {
		return 0
	}
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	curr := t.root
	for _, k := range prefix {
		child, exists := curr.children[k]
		if !exists {
			return 0
		}
		curr = child
	}
	return curr.count
}

// Range call "fn" for each keys-value pair in the trie.
// If "fn" returns false, stops the iteration.
func (t *Trie[K, V]) Range(fn func([]K, V) bool) {
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

	path := make([]K, 0, t.size)
	t._range(t.root, path, fn)
}

func (t *Trie[K, V]) _range(node *Node[K, V], path []K, fn func([]K, V) bool) bool {
	if node.hasValue {
		fullPath := make([]K, len(path))
		copy(fullPath, path)
		if !fn(fullPath, node.value) {
			return false
		}
	}
	for k, child := range node.children {
		path = append(path, k)
		if !t._range(child, path, fn) {
			return false
		}
		path = path[:len(path)-1] // backtrack and remove the last key
	}
	return true
}

// LongestPrefix returns the longest prefix of the given keys that exists in the trie,
// along with its associated value and a boolean indicating if such a prefix was found.
//
// For example, if the trie contains the following key-value pairs:
//   - "internal" -> "A"
//   - "inter" -> "B"
//   - "in" -> "C"
//
// LongestPrefix("internally") would return ("internal", "A", true)
// LongestPrefix("inter") would return ("inter", "B", true)
// LongestPrefix("int") would return ("in", "C", true)
// LongestPrefix("foo") would return (nil, zero_value, false)
//
// This is particularly useful in scenarios like:
//  1. Router matching: finding the most specific route that matches a URL
//  2. IP routing: finding the most specific network prefix for an IP
//  3. Word segmentation: finding the longest dictionary word in a string
func (t *Trie[K, V]) LongestPrefix(keys []K) ([]K, V, bool) {
	if len(keys) == 0 {
		return nil, *new(V), false
	}
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	if t.root == nil {
		return nil, *new(V), false
	}

	curr := t.root
	longestPrefix := make([]K, 0)
	longestValue := *new(V)
	found := false

	for i, k := range keys {
		child, exists := curr.children[k]
		if !exists {
			break
		}
		curr = child
		if curr.hasValue {
			longestPrefix = keys[:i+1]
			longestValue = curr.value
			found = true
		}
	}

	return longestPrefix, longestValue, found
}

// PathAncestors returns all ancestor nodes (including the target node) from root to the given path.
// This is useful for collecting parameters from all levels in a hierarchical structure.
// Returns a slice of KeysValue where each entry represents an ancestor node with its path and value.
func (t *Trie[K, V]) PathAncestors(keys []K) []KeysValue[K, V] {
	if len(keys) == 0 {
		return nil
	}
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	if t.root == nil {
		return nil
	}

	ancestors := make([]KeysValue[K, V], 0)
	curr := t.root
	currentPath := make([]K, 0)

	// Check if root has value
	if curr.hasValue {
		ancestors = append(ancestors, KeysValue[K, V]{
			Keys:  make([]K, 0), // empty path for root
			Value: curr.value,
		})
	}

	// Traverse the path and collect all ancestors with values
	for _, k := range keys {
		child, exists := curr.children[k]
		if !exists {
			break // Path doesn't exist, stop here
		}
		curr = child
		currentPath = append(currentPath, k)

		// If this node has a value, add it to ancestors
		if curr.hasValue {
			pathCopy := make([]K, len(currentPath))
			copy(pathCopy, currentPath)
			ancestors = append(ancestors, KeysValue[K, V]{
				Keys:  pathCopy,
				Value: curr.value,
			})
		}
	}

	return ancestors
}

// String returns a string representation of the trie.
func (t *Trie[K, V]) String() string {
	if t.safe {
		t.mu.RLock()
		defer t.mu.RUnlock()
	}

	if t.root == nil {
		return "Trie (empty)"
	}
	nodeFormatter := t.nodeFormatter
	if nodeFormatter == nil {
		nodeFormatter = nodeString[V]
	}
	keyFormatter := t.keyFormatter
	if keyFormatter == nil {
		keyFormatter = func(k K, v V, count int, hasValue bool) string {
			if count > 1 {
				return fmt.Sprintf("%v(%d)", k, count)
			}
			return fmt.Sprintf("%v", k)
		}
	}
	var sb strings.Builder
	sb.WriteString("Trie\n")
	t.output(t.root, " ", "", &sb, nodeFormatter, keyFormatter)
	return sb.String()
}

func (t *Trie[K, V]) output(node *Node[K, V],
	valuePrefix string, childPrefix string, sb *strings.Builder,
	nodeFormatter func(V, int, bool) string, keyFormatter func(K, V, int, bool) string,
) {
	sb.WriteString(valuePrefix)

	// output current node.
	sb.WriteString(nodeFormatter(node.value, node.count, node.hasValue))
	sb.WriteString("\n")

	// collect all children.
	children := make([]struct {
		key  K
		node *Node[K, V]
	}, 0, len(node.children))
	for k, child := range node.children {
		children = append(children, struct {
			key  K
			node *Node[K, V]
		}{k, child})
	}

	sort.Slice(children, func(i, j int) bool {
		return fmt.Sprint(children[i].key) > fmt.Sprint(children[j].key)
	})

	// iterate over children
	for i, child := range children {
		isLast := i == len(children)-1

		// determine connection line and child prefix
		newPrefix := childPrefix
		if isLast {
			sb.WriteString(childPrefix + "└─" + keyFormatter(child.key, child.node.value, child.node.count, child.node.hasValue))
			newPrefix += "  "
		} else {
			sb.WriteString(childPrefix + "├─" + keyFormatter(child.key, child.node.value, child.node.count, child.node.hasValue))
			newPrefix += "│ "
		}

		t.output(child.node, valuePrefix, newPrefix, sb, nodeFormatter, keyFormatter)
	}
}

func nodeString[V any](v V, _ int, hasValue bool) string {
	if hasValue {
		return fmt.Sprintf("● %v", v)
		// return fmt.Sprintf("%v", n.value)
	}
	// return ""
	return "○"
}
