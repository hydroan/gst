package splaytree_test

import (
	"cmp"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/hydroan/gst/ds/tree/splaytree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newIntStringTree(t *testing.T) *splaytree.Tree[int, string] {
	t.Helper()
	tree, err := splaytree.NewOrderedKeys(splaytree.WithSafe[int, string]())
	require.NoError(t, err)
	return tree
}

func TestSplayTree_New(t *testing.T) {
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
			tree, err := splaytree.New[int, int](tt.cmp)
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

func TestSplayTree_Put(t *testing.T) {
	tree := newIntStringTree(t)

	// Test empty tree insert
	tree.Put(1, "one")
	assert.Equal(t, 1, tree.Size())

	// Test update existing key
	tree.Put(1, "ONE")
	assert.Equal(t, 1, tree.Size())
	val, exists := tree.Get(1)
	assert.True(t, exists)
	assert.Equal(t, "ONE", val)

	// Test multiple inserts
	tree.Put(2, "two")
	tree.Put(3, "three")
	assert.Equal(t, 3, tree.Size())
}

func TestSplayTree_Get(t *testing.T) {
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

func TestSplayTree_Delete(t *testing.T) {
	tree := newIntStringTree(t)

	// Test delete from empty tree
	val, exists := tree.Delete(1)
	assert.False(t, exists)
	assert.Empty(t, val)

	// Test delete non-existent key
	tree.Put(1, "one")
	val, exists = tree.Delete(2)
	assert.False(t, exists)
	assert.Empty(t, val)
	assert.Equal(t, 1, tree.Size())

	tree.Put(4, "four")
	tree.Put(2, "two")
	tree.Put(6, "six")
	tree.Put(1, "one")
	tree.Put(3, "three")
	tree.Put(5, "five")
	tree.Put(7, "seven")

	// Test delete leaf node
	val, exists = tree.Delete(7)
	assert.True(t, exists)
	assert.Equal(t, "seven", val)
	assert.Equal(t, 6, tree.Size())

	// Test delete node with one child
	val, exists = tree.Delete(6)
	assert.True(t, exists)
	assert.Equal(t, "six", val)
	assert.Equal(t, 5, tree.Size())

	// Test delete node with two children
	val, exists = tree.Delete(2)
	assert.True(t, exists)
	assert.Equal(t, "two", val)
	assert.Equal(t, 4, tree.Size())

	// Delete remaining nodes
	tree.Delete(1)
	tree.Delete(3)
	tree.Delete(4)
	tree.Delete(5)

	// Verify tree is empty
	assert.True(t, tree.IsEmpty())

	// Test consecutive deletes don't panic
	tree.Delete(1)
	tree.Delete(1)
}

func TestSplayTree_MinMax(t *testing.T) {
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

func TestSplayTree_FloorCeiling(t *testing.T) {
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

	// Test empty tree
	t.Run("Empty tree Floor", func(t *testing.T) {
		emptyTree := newIntStringTree(t)
		k, v, found := emptyTree.Floor(5)
		assert.False(t, found)
		assert.Zero(t, k)
		assert.Empty(t, v)
	})

	t.Run("Empty tree Ceiling", func(t *testing.T) {
		emptyTree := newIntStringTree(t)
		k, v, found := emptyTree.Ceiling(5)
		assert.False(t, found)
		assert.Zero(t, k)
		assert.Empty(t, v)
	})
}

func TestSplayTree_Traversal(t *testing.T) {
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
		// fmt.Println(tree)
		// tree.Get(1)
		// fmt.Println(tree)
		// tree.Get(2)
		// fmt.Println(tree)
		// tree.Get(3)
		// fmt.Println(tree)
		// tree.Get(4)
		// fmt.Println(tree)
		// tree.Get(5)
		// fmt.Println(tree)
		// tree.Get(6)
		// fmt.Println(tree)
		// tree.Get(7)
		// fmt.Println(tree)
		// tree.Get(8)
		// fmt.Println(tree)
		// tree.Get(9)
		// fmt.Println(tree)
		// tree.Get(10)
		// fmt.Println(tree)
		// tree.Get(5)
		// fmt.Println(tree)
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

func TestSplayTree_MarshalJSON(t *testing.T) {
	tree := newIntStringTree(t)
	for i := range 10 {
		tree.Put(i, strconv.Itoa(i))
	}
	jsonBytes, err := json.Marshal(tree)
	require.NoError(t, err)
	// fmt.Println(string(jsonBytes))

	tree2 := newIntStringTree(t)
	err = json.Unmarshal(jsonBytes, tree2)
	require.NoError(t, err)
	assert.Equal(t, tree.Keys(), tree2.Keys())
	assert.Equal(t, tree.Values(), tree2.Values())
}

// func TestSplayTree_String(t *testing.T) {
// 	tree, err := splaytree.NewWithOrderedKeys(splaytree.WithSafe[int, string](), splaytree.WithNodeFormat[int, string]("%d(%s)"))
// 	assert.NoError(t, err)
//
// 	for i := range 10 {
// 		tree.Put(i, fmt.Sprintf("%d", i))
// 		fmt.Println(tree)
// 	}
// }
