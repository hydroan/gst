package skiplist

import (
	"cmp"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/hydroan/gst/ds/types"
)

const (
	defaultMaxLevel    = 20
	defaultProbability = 0.25
)

// Node represents an element in the SkipList, storing a key-value pair and multiple level pointers.
type Node[K comparable, V any] struct {
	Key   K // Key of the node
	Value V // Associated value of the node

	// next stores pointers to the next nodes at different levels.
	// The length of next is determined when the node is created and equals its assigned level.
	//
	// Example structure:
	//
	// Level 3:        ───────(20)───────────────(50)
	//                    │
	//                    ▼
	// Level 2:        ───(10)──────(20)──(35)──────(50)──(70)
	//                             │      │
	//                             ▼      ▼
	// Level 1:   (5)──(10)──(15)──(20)──(30)──(35)──(40)──(50)──(60)──(70)──(80)
	//
	// For example, if n := Node(20), its next pointers might be:
	//	n.next = []*Node{Node(30), Node(35), Node(50)}
	//	n.next[0] = Node(30) // Level 1
	//	n.next[1] = Node(35) // Level 2
	//	n.next[2] = Node(50) // Level 3
	//
	// Each level allows fast traversal across multiple nodes.
	next []*Node[K, V] // Pointers to the next nodes at different levels
}

// SkipList represents a probabilistic balanced data structure for fast lookups, insertions, and deletions.
// It maintains multiple levels of linked lists, allowing logarithmic time complexity (O(log n)) for operations.
type SkipList[K comparable, V any] struct {
	head *Node[K, V] // Sentinel head node, does not hold any key-value pair

	// The SkipList structure is organized as follows:
	// Level 3:              (20) --------- (50)
	// Level 2:       (10) -- (20) --- (35) -- (50) -- (70)
	// Level 1: (5) -- (10) -- (15) -- (20) -- (30) -- (35) -- (40) -- (50) -- (60) -- (70) -- (80)
	//
	// level    = 3  // Current highest level in the skip list
	// maxLevel = defaultMaxLevel  // Maximum level a node can reach
	// size     = 11 // Number of key-value pairs in the skip list

	level    int     // Current highest level in the skip list (0-based index)
	maxLevel int     // Maximum allowed level (typically log(size))
	size     int     // Total number of elements stored in the skip list
	p        float64 // Probability factor for determining node levels (commonly 0.5)

	rand *rand.Rand     // Random number generator for level assignment
	cmp  func(K, K) int // Comparison function for ordering keys

	safe bool         // Whether the skip list is thread-safe
	mu   types.Locker // Mutex for synchronization (used if safe=true)

	nodeFormatter func(K, V) string
}

// New creates a new skiplist with the given comparison function and optional parameters.
func New[K comparable, V any](cmp func(K, K) int, ops ...Option[K, V]) (*SkipList[K, V], error) {
	if cmp == nil {
		return nil, types.ErrComparisonNil
	}
	sl := &SkipList[K, V]{
		head:     &Node[K, V]{next: make([]*Node[K, V], defaultMaxLevel)},
		level:    1,
		maxLevel: defaultMaxLevel,
		p:        defaultProbability,
		cmp:      cmp,
		rand:     rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano()))), // #nosec G404 G115
	}
	for _, op := range ops {
		if op == nil {
			continue
		}
		if err := op(sl); err != nil {
			return nil, err
		}
	}
	return sl, nil
}

// NewOrdered is a convenience function of New that uses cmp.Ordered as the comparison function.
func NewOrdered[K cmp.Ordered, V any](ops ...Option[K, V]) (*SkipList[K, V], error) {
	return New(cmp.Compare, ops...)
}

// NewFromMap creates a new skiplist from the given map.
func NewFromMap[K comparable, V any](cmp func(K, K) int, m map[K]V, ops ...Option[K, V]) (*SkipList[K, V], error) {
	sl, err := New(cmp, ops...)
	if err != nil {
		return nil, err
	}
	for k, v := range m {
		sl.Put(k, v)
	}
	return sl, nil
}

// NewFromOrderedMap is a convenience function of NewFromMap that uses cmp.Ordered as the comparison function.
func NewFromOrderedMap[K cmp.Ordered, V any](m map[K]V, ops ...Option[K, V]) (*SkipList[K, V], error) {
	return NewFromMap(cmp.Compare, m, ops...)
}

// Put inserts a key-value pair into the skip list.
// If the key already exists, its value is updated.
func (sl *SkipList[K, V]) Put(key K, value V) {
	if sl.safe {
		sl.mu.Lock()
		defer sl.mu.Unlock()
	}

	sl.put(key, value)
}

func (sl *SkipList[K, V]) put(key K, value V) {
	// Trace update positions at each level.
	update := make([]*Node[K, V], sl.maxLevel)
	curr := sl.head

	// Search for the insertion point from the highest level down to level 0.
	for i := sl.level - 1; i >= 0; i-- {
		for curr.next[i] != nil && sl.cmp(curr.next[i].Key, key) < 0 {
			curr = curr.next[i]
		}
		update[i] = curr
	}

	// Updated it if already exists.
	if curr.next[0] != nil && sl.cmp(curr.next[0].Key, key) == 0 {
		curr.next[0].Value = value
		return
	}

	// Generate a new level for the node using probability factor `p`
	nodeLevel := sl.randomLevel()
	newNode := &Node[K, V]{
		Key:   key,
		Value: value,
		next:  make([]*Node[K, V], nodeLevel),
	}

	// If new node's level is greater than current max level, extend list height.
	if nodeLevel > sl.level {
		for i := sl.level; i < nodeLevel; i++ {
			update[i] = sl.head
		}
		sl.level = nodeLevel
	}

	// Insert new node by updating forward pointers
	for i := range nodeLevel {
		newNode.next[i] = update[i].next[i]
		update[i].next[i] = newNode
	}

	sl.size++
}

// Get searches for a key in the SkipList and returns its associated value.
// If the key is found, it returns the value and true; otherwise, it returns the zero value and false.
func (sl *SkipList[K, V]) Get(key K) (v V, found bool) {
	if sl.safe {
		sl.mu.RLock()
		defer sl.mu.RUnlock()
	}
	if sl.size == 0 {
		return v, false
	}

	curr := sl.head
	// Traverse from the highest level down to level 0
	for i := sl.level - 1; i >= 0; i-- {
		// Move forward while the next node exists and its key is less than the target key
		for curr.next[i] != nil && sl.cmp(curr.next[i].Key, key) < 0 {
			curr = curr.next[i]
		}
	}

	// Move to the potential target node at level 0
	curr = curr.next[0]
	if curr != nil && sl.cmp(curr.Key, key) == 0 {
		return curr.Value, true
	}
	return v, false
}

// Delete removes a key-value pair from the skiplist.
// Returns the delted value and true if it exists, zero value and false otherwise.
func (sl *SkipList[K, V]) Delete(key K) (v V, found bool) {
	if sl.safe {
		sl.mu.Lock()
		defer sl.mu.Unlock()
	}
	if sl.size == 0 {
		return v, false
	}

	update := make([]*Node[K, V], sl.level) // Stores previous nodes at each level
	curr := sl.head

	// Traverse from the highest level down to level 0 to find the node
	for i := sl.level - 1; i >= 0; i-- {
		for curr.next[i] != nil && sl.cmp(curr.next[i].Key, key) < 0 {
			curr = curr.next[i]
		}
		update[i] = curr // Store the last node before the target node at each level
	}

	// Move to the possible target node at level 0
	target := curr.next[0]
	if target == nil || sl.cmp(target.Key, key) != 0 {
		return v, false // Key not found
	}

	// Store value to return before deletion
	deletedValue := target.Value

	// Remove target node from all levels
	for i := range len(target.next) {
		if update[i].next[i] == target {
			update[i].next[i] = target.next[i] // Bypass target node
		}
	}

	// Reduce level if upper levels are empty
	for sl.level > 1 && sl.head.next[sl.level-1] == nil {
		sl.level--
	}

	sl.size--

	return deletedValue, true
}

// IsEmpty reports whether the skiplist is empty.
func (sl *SkipList[K, V]) IsEmpty() bool {
	if sl.safe {
		sl.mu.RLock()
		defer sl.mu.RUnlock()
	}

	return sl.size == 0
}

// Len returns the number of elements in the skiplist.
func (sl *SkipList[K, V]) Len() int {
	if sl.safe {
		sl.mu.Lock()
		defer sl.mu.Unlock()
	}

	return sl.size
}

// Clear removes all keys-value pairs from the skiplist.
func (sl *SkipList[K, V]) Clear() {
	if sl.safe {
		sl.mu.Lock()
		defer sl.mu.Unlock()
	}
	if sl.size == 0 {
		return
	}

	// Reset head's next pointers to nil
	for i := range sl.head.next {
		sl.head.next[i] = nil
	}

	sl.level = 1
	sl.size = 0
}

// Min returns the minimum key-value pair in the skiplist.
// Returns zero value and false if the skiplist is empty.
func (sl *SkipList[K, V]) Min() (k K, v V, found bool) {
	if sl.safe {
		sl.mu.RLock()
		defer sl.mu.RUnlock()
	}
	if sl.size == 0 {
		return k, v, false
	}

	return sl.head.next[0].Key, sl.head.next[0].Value, true
}

// Max returns the maximum key-value pair in the skiplist.
// Returns zero value and false if the skiplist is empty.
func (sl *SkipList[K, V]) Max() (k K, v V, found bool) {
	if sl.safe {
		sl.mu.RLock()
		defer sl.mu.RUnlock()
	}
	if sl.size == 0 {
		return k, v, false
	}

	curr := sl.head
	for level := sl.level - 1; level >= 0; level-- {
		for curr.next[level] != nil {
			curr = curr.next[level]
		}
	}
	if curr == sl.head {
		return k, v, false
	}
	return curr.Key, curr.Value, true
}

// Floor find the largest key less than or equal to the given key.
// Returns the key-value pair and true if found, zero value and false if not found.
func (sl *SkipList[K, V]) Floor(key K) (k K, v V, found bool) {
	if sl.safe {
		sl.mu.RLock()
		defer sl.mu.RUnlock()
	}
	if sl.size == 0 {
		return k, v, false
	}

	curr := sl.head
	for i := sl.level - 1; i >= 0; i-- {
		for curr.next[i] != nil && sl.cmp(curr.next[i].Key, key) <= 0 {
			curr = curr.next[i]
		}
	}
	if curr == sl.head {
		return k, v, false
	}
	return curr.Key, curr.Value, true
}

// Ceiling find the smallest key greater than or equal to the given key.
// Returns the key-value pair and true if found, zero value and false if not found.
func (sl *SkipList[K, V]) Ceiling(key K) (k K, v V, found bool) {
	if sl.safe {
		sl.mu.RLock()
		defer sl.mu.RUnlock()
	}

	if sl.size == 0 {
		return k, v, false
	}
	curr := sl.head
	for i := sl.level - 1; i >= 0; i-- {
		for curr.next[i] != nil && sl.cmp(curr.next[i].Key, key) < 0 {
			curr = curr.next[i]
		}
	}
	if curr.next[0] == nil {
		return k, v, false
	}
	return curr.next[0].Key, curr.next[0].Value, true
}

// Keys returns all keys in the skiplist.
// Returns empty slice(not nil) if the skiplist is empty
func (sl *SkipList[K, V]) Keys() []K {
	if sl.safe {
		sl.mu.RLock()
		defer sl.mu.RUnlock()
	}

	keys := make([]K, 0, sl.size)
	curr := sl.head.next[0]
	for curr != nil {
		keys = append(keys, curr.Key)
		curr = curr.next[0]
	}
	return keys
}

// Values returns all values in the skiplist.
// Returns empty slice(not nil) if the skiplist is empty
func (sl *SkipList[K, V]) Values() []V {
	if sl.safe {
		sl.mu.RLock()
		defer sl.mu.RUnlock()
	}

	values := make([]V, 0, sl.size)
	curr := sl.head.next[0]
	for curr != nil {
		values = append(values, curr.Value)
		curr = curr.next[0]
	}
	return values
}

// Range iterates over all key-value pairs in the SkipList in ascending order.
// The function `fn` is called for each key-value pair.
// If `fn` returns false, iteration stops early.
func (sl *SkipList[K, V]) Range(fn func(K, V) bool) {
	if fn == nil {
		return
	}
	if sl.safe {
		sl.mu.RLock()
		defer sl.mu.RUnlock()
	}

	curr := sl.head.next[0]
	for curr != nil {
		if !fn(curr.Key, curr.Value) {
			break
		}
		curr = curr.next[0] // Move to the next node at level 0
	}
}

// Clone returns a deep copy of the skiplist.
func (sl *SkipList[K, V]) Clone() *SkipList[K, V] {
	if sl.safe {
		sl.mu.RLock()
		defer sl.mu.RUnlock()
	}

	clone, _ := New(sl.cmp, sl.options()...)
	curr := sl.head.next[0]
	for curr != nil {
		clone.Put(curr.Key, curr.Value)
		curr = curr.next[0]
	}
	return clone
}

func (sl *SkipList[K, V]) options() []Option[K, V] {
	ops := make([]Option[K, V], 0)
	if sl.safe {
		ops = append(ops, WithSafe[K, V]())
	}
	if sl.nodeFormatter != nil {
		ops = append(ops, WithNodeFormatter(sl.nodeFormatter))
	}
	ops = append(
		ops,
		WithMaxLevel[K, V](sl.maxLevel),
		WithProbability[K, V](sl.p),
	)
	return ops
}

// String returns a string representation of the skiplist.
func (sl *SkipList[K, V]) String() string {
	if sl.safe {
		sl.mu.RLock()
		defer sl.mu.RUnlock()
	}

	if sl.size == 0 {
		return "SkipList is empty"
	}

	var sb strings.Builder
	sb.WriteString("SkipList Structure:\n")
	formatter := sl.nodeFormatter
	if formatter == nil {
		formatter = func(k K, _ V) string { return fmt.Sprintf("%v", k) }
	}

	for i := sl.level - 1; i >= 0; i-- {
		fmt.Fprintf(&sb, "Level %d: ", i)
		curr := sl.head.next[i]
		for curr != nil {
			fmt.Fprintf(&sb, "%v -> ", formatter(curr.Key, curr.Value))
			curr = curr.next[i]
		}
		sb.WriteString("nil\n")
	}

	return sb.String()
}

// randomLevel generates a level for the new node using probability `p`.
// It ensures that higher levels occur exponentially less frequently.
func (sl *SkipList[K, V]) randomLevel() int {
	level := 1
	for level < sl.maxLevel && sl.rand.Float64() < sl.p {
		level++
	}
	return level
}
