package avltree_test

import (
	"cmp"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/hydroan/gst/ds/tree/avltree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newIntStringTree(t *testing.T) *avltree.Tree[int, string] {
	t.Helper()
	tree, err := avltree.NewOrderedKeys(avltree.WithSafe[int, string]())
	require.NoError(t, err)
	return tree
}

func TestAVLTree_New(t *testing.T) {
	tests := []struct {
		name    string
		cmp     func(int, int) int
		wantErr bool
	}{
		{
			name:    "nil comparator",
			cmp:     nil,
			wantErr: true,
		},
		{
			name:    "valid comparator",
			cmp:     cmp.Compare[int],
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := avltree.New[int, int](tt.cmp)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, tree)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, tree)
			}
		})
	}
}

func TestAVLTree_Put(t *testing.T) {
	tree := newIntStringTree(t)

	// Test empty tree insert
	tree.Put(1, "one")
	assert.Equal(t, 1, tree.Size())
	assert.Equal(t, 1, tree.Height())

	// Test update existing key
	tree.Put(1, "ONE")
	assert.Equal(t, 1, tree.Size())
	val, exists := tree.Get(1)
	assert.True(t, exists)
	assert.Equal(t, "ONE", val)

	// Test multiple inserts with balancing
	// Test LL rotation
	tree.Put(2, "two")
	tree.Put(3, "three") // Should trigger right rotation
	assert.Equal(t, 3, tree.Size())
	assert.Equal(t, 2, tree.Height())

	// Test RR rotation
	tree.Put(0, "zero")
	tree.Put(-1, "minus one") // Should trigger left rotation
	assert.Equal(t, 5, tree.Size())
	assert.Equal(t, 3, tree.Height())

	// Test LR rotation
	tree.Put(7, "seven")
	tree.Put(5, "five")
	tree.Put(6, "six") // Should trigger left-right rotation
	assert.Equal(t, 8, tree.Size())
	assert.Equal(t, 4, tree.Height())

	// Test RL rotation
	tree.Put(10, "ten")
	tree.Put(8, "eight")
	tree.Put(9, "nine") // Should trigger right-left rotation
	assert.Equal(t, 11, tree.Size())
	assert.Equal(t, 4, tree.Height())
}

func TestAVLTree_Get(t *testing.T) {
	tree := newIntStringTree(t)

	// Test get from empty tree
	val, exists := tree.Get(1)
	assert.False(t, exists)
	assert.Empty(t, val)

	// Test existing key
	tree.Put(1, "one")
	val, exists = tree.Get(1)
	assert.True(t, exists)
	assert.Equal(t, "one", val)

	// Test non-existing key
	val, exists = tree.Get(2)
	assert.False(t, exists)
	assert.Empty(t, val)
}

func TestAVLTree_Delete(t *testing.T) {
	tree := newIntStringTree(t)

	// Test delete from empty tree
	assert.Equal(t, 0, tree.Size())

	// Setup test tree
	values := []struct {
		key int
		val string
	}{
		{4, "four"},
		{2, "two"},
		{6, "six"},
		{1, "one"},
		{3, "three"},
		{5, "five"},
		{7, "seven"},
	}

	for _, v := range values {
		tree.Put(v.key, v.val)
	}
	assert.Equal(t, 7, tree.Size())

	// Test delete leaf node
	tree.Delete(7)
	assert.Equal(t, 6, tree.Size())

	// Test delete node with one child
	tree.Delete(6)
	assert.Equal(t, 5, tree.Size())

	// Test delete node with two children
	tree.Delete(2)
	assert.Equal(t, 4, tree.Size())

	// Delete remaining nodes
	tree.Delete(1)
	tree.Delete(3)
	tree.Delete(4)
	tree.Delete(5)

	// Verify tree is empty
	assert.True(t, tree.IsEmpty())
}

func TestAVLTree_MinMax(t *testing.T) {
	tree := newIntStringTree(t)

	// Test empty tree
	k, v, exists := tree.Min()
	assert.False(t, exists)
	assert.Zero(t, k)
	assert.Empty(t, v)

	k, v, exists = tree.Max()
	assert.False(t, exists)
	assert.Zero(t, k)
	assert.Empty(t, v)

	// Setup tree
	values := map[int]string{
		1: "one",
		3: "three",
		2: "two",
	}
	for k, v := range values {
		tree.Put(k, v)
	}

	// Test min
	k, v, exists = tree.Min()
	assert.True(t, exists)
	assert.Equal(t, 1, k)
	assert.Equal(t, "one", v)

	// Test max
	k, v, exists = tree.Max()
	assert.True(t, exists)
	assert.Equal(t, 3, k)
	assert.Equal(t, "three", v)
}

func TestAVLTree_FloorCeiling(t *testing.T) {
	tree := newIntStringTree(t)
	values := map[int]string{
		1:  "one",
		3:  "three",
		5:  "five",
		7:  "seven",
		9:  "nine",
		11: "eleven",
	}
	for k, v := range values {
		tree.Put(k, v)
	}

	// Test Floor
	tests := []struct {
		name      string
		input     int
		wantKey   int
		wantValue string
		wantFound bool
	}{
		{"Floor of minimum - 1", 0, 0, "", false},
		{"Floor of existing key", 5, 5, "five", true},
		{"Floor between keys", 6, 5, "five", true},
		{"Floor of maximum + 1", 12, 11, "eleven", true},
		{"Floor between 3 and 5", 4, 3, "three", true},
		{"Floor between 7 and 9", 8, 7, "seven", true},
	}

	for _, tt := range tests {
		t.Run("Floor_"+tt.name, func(t *testing.T) {
			k, v, found := tree.Floor(tt.input)
			assert.Equal(t, tt.wantFound, found)
			if found {
				assert.Equal(t, tt.wantKey, k)
				assert.Equal(t, tt.wantValue, v)
			}
		})
	}

	// Test Ceiling
	ceilingTests := []struct {
		name      string
		input     int
		wantKey   int
		wantValue string
		wantFound bool
	}{
		{"Ceiling of minimum - 1", 0, 1, "one", true},
		{"Ceiling of existing key", 5, 5, "five", true},
		{"Ceiling between keys", 6, 7, "seven", true},
		{"Ceiling of maximum + 1", 12, 0, "", false},
		{"Ceiling between 3 and 5", 4, 5, "five", true},
		{"Ceiling between 7 and 9", 8, 9, "nine", true},
	}

	for _, tt := range ceilingTests {
		t.Run("Ceiling_"+tt.name, func(t *testing.T) {
			k, v, found := tree.Ceiling(tt.input)
			assert.Equal(t, tt.wantFound, found)
			if found {
				assert.Equal(t, tt.wantKey, k)
				assert.Equal(t, tt.wantValue, v)
			}
		})
	}
}

func TestAVLTree_Traversal(t *testing.T) {
	tree := newIntStringTree(t)
	values := map[int]string{
		1:  "one",
		2:  "two",
		3:  "three",
		4:  "four",
		5:  "five",
		6:  "six",
		7:  "seven",
		8:  "eight",
		9:  "nine",
		10: "ten",
	}
	for k, v := range values {
		tree.Put(k, v)
	}

	t.Run("InOrder", func(t *testing.T) {
		result := make([]int, 0)
		tree.InOrder(func(key int, value string) bool {
			result = append(result, key)
			return true
		})
		expected := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		assert.Equal(t, expected, result)
	})

	t.Run("PreOrder", func(t *testing.T) {
		result := make([]int, 0)
		tree.PreOrder(func(key int, value string) bool {
			result = append(result, key)
			return true
		})
		assert.Len(t, result, len(values))
	})

	t.Run("PostOrder", func(t *testing.T) {
		result := make([]int, 0)
		tree.PostOrder(func(key int, value string) bool {
			result = append(result, key)
			return true
		})
		assert.Len(t, result, len(values))
	})

	t.Run("LevelOrder", func(t *testing.T) {
		result := make([]int, 0)
		tree.LevelOrder(func(key int, value string) bool {
			result = append(result, key)
			return true
		})
		assert.Len(t, result, len(values))
	})

	t.Run("Early termination", func(t *testing.T) {
		result := make([]int, 0)
		tree.InOrder(func(key int, value string) bool {
			result = append(result, key)
			return key < 5
		})
		expected := []int{1, 2, 3, 4, 5}
		assert.Equal(t, expected, result)
	})
}

func TestAVLTree_String(t *testing.T) {
	m := map[string]int{
		"one":   1,
		"two":   2,
		"three": 3,
		"four":  4,
		"five":  5,
		"six":   6,
		"seven": 7,
		"eight": 8,
		"nine":  9,
		"ten":   10,
	}
	tt, err := avltree.NewFromOrderedMap(m)
	require.NoError(t, err)
	fmt.Println(tt.String())
	tt, err = avltree.NewFromOrderedMap(m, avltree.WithNodeFormatter(func(k string, v int) string {
		return strconv.Itoa(v)
	}))
	require.NoError(t, err)
	fmt.Println(tt.String())
}

func TestAVLTree_MarshalJSON(t *testing.T) {
	tree := newIntStringTree(t)
	for i := range 10 {
		tree.Put(i, strconv.Itoa(i))
	}
	jsonBytes, err := json.Marshal(tree)
	require.NoError(t, err)

	tree2 := newIntStringTree(t)
	err = json.Unmarshal(jsonBytes, tree2)
	require.NoError(t, err)
	assert.Equal(t, tree.Keys(), tree2.Keys())
	assert.Equal(t, tree.Values(), tree2.Values())
}
