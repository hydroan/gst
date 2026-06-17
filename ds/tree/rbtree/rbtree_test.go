package rbtree_test

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/ds/tree/rbtree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//
// import (
// 	"fmt"
// 	"math/rand/v2"
// 	"testing"
// 	"time"
//
// 	"github.com/hydroan/gst/ds/tree/rbtree"
// 	"github.com/stretchr/testify/assert"
// )
//
// func intComparator(a, b int) int {
// 	switch {
// 	case a < b:
// 		return -1
// 	case a > b:
// 		return 1
// 	default:
// 		return 0
// 	}
// }
//
// func TestRedBlackTree_New(t *testing.T) {
// 	// 测试 New
// 	tree, err := rbtree.New[int, string](intComparator)
// 	assert.NoError(t, err)
// 	assert.NotNil(t, tree)
// 	assert.True(t, tree.IsEmpty())
// 	assert.Equal(t, 0, tree.Size())
//
// 	// 测试 NewWithOrderedKeys
// 	orderedTree, err := rbtree.NewWithOrderedKeys[int, string]()
// 	assert.NoError(t, err)
// 	assert.NotNil(t, orderedTree)
// 	assert.True(t, orderedTree.IsEmpty())
// 	assert.Equal(t, 0, orderedTree.Size())
//
// 	// 测试 NewFromMap
// 	m := map[int]string{1: "one", 2: "two", 3: "three"}
// 	treeFromMap, err := rbtree.NewFromMap(m, intComparator)
// 	assert.NoError(t, err)
// 	assert.NotNil(t, treeFromMap)
// 	assert.False(t, treeFromMap.IsEmpty())
// 	assert.Equal(t, len(m), treeFromMap.Size())
//
// 	// 测试 NewFromSlice
// 	slice := []string{"zero", "one", "two", "three"}
// 	sliceTree, err := rbtree.NewFromSlice(slice)
// 	assert.NoError(t, err)
// 	assert.NotNil(t, sliceTree)
// 	assert.False(t, sliceTree.IsEmpty())
// 	assert.Equal(t, len(slice), sliceTree.Size())
//
// 	fmt.Println(sliceTree)
//
// 	// 验证 map 中的 key 都存在于树中
// 	for k, v := range m {
// 		val, found := treeFromMap.Get(k)
// 		assert.True(t, found)
// 		assert.Equal(t, v, val)
// 	}
//
// 	// 测试 NewFromMapWithOrderedKeys
// 	orderedTreeFromMap, err := rbtree.NewFromMapWithOrderedKeys(m)
// 	assert.NoError(t, err)
// 	assert.NotNil(t, orderedTreeFromMap)
// 	assert.False(t, orderedTreeFromMap.IsEmpty())
// 	assert.Equal(t, len(m), orderedTreeFromMap.Size())
//
// 	// 验证 map 中的 key 都存在于树中
// 	for k, v := range m {
// 		val, found := orderedTreeFromMap.Get(k)
// 		assert.True(t, found)
// 		assert.Equal(t, v, val)
// 	}
// }
//
// func TestRedBlackTree_BasicOperations(t *testing.T) {
// 	tree, err := rbtree.New[int, string](intComparator)
// 	assert.NoError(t, err)
//
// 	// 测试空树
// 	assert.True(t, tree.IsEmpty())
// 	assert.Equal(t, 0, tree.Size())
//
// 	// 插入元素
// 	tree.Put(10, "ten")
// 	tree.Put(20, "twenty")
// 	tree.Put(5, "five")
//
// 	assert.False(t, tree.IsEmpty())
// 	assert.Equal(t, 3, tree.Size())
//
// 	// 获取元素
// 	val, found := tree.Get(10)
// 	assert.True(t, found)
// 	assert.Equal(t, "ten", val)
//
// 	// 获取不存在的元素
// 	val, found = tree.Get(100)
// 	assert.False(t, found)
// 	assert.Equal(t, "", val)
//
// 	// 删除元素
// 	tree.Delete(10)
// 	_, found = tree.Get(10)
// 	assert.False(t, found)
// 	assert.Equal(t, 2, tree.Size())
//
// 	// 删除不存在的元素
// 	tree.Delete(100) // 不应引发错误
// 	assert.Equal(t, 2, tree.Size())
// }
//
// func TestRedBlackTree_MinMax(t *testing.T) {
// 	tree, err := rbtree.New[int, string](intComparator)
// 	assert.NoError(t, err)
//
// 	// 测试空树
// 	assert.Nil(t, tree.Min())
// 	assert.Nil(t, tree.Max())
//
// 	// 插入元素
// 	tree.Put(15, "fifteen")
// 	tree.Put(10, "ten")
// 	tree.Put(20, "twenty")
// 	tree.Put(5, "five")
//
// 	// 最小值
// 	assert.Equal(t, 5, tree.Min().Key)
// 	assert.Equal(t, "five", tree.Min().Value)
//
// 	// 最大值
// 	assert.Equal(t, 20, tree.Max().Key)
// 	assert.Equal(t, "twenty", tree.Max().Value)
// }
//
// func TestRedBlackTree_FloorCeiling(t *testing.T) {
// 	tree, err := rbtree.New[int, string](intComparator)
// 	assert.NoError(t, err)
// 	tree.Put(10, "ten")
// 	tree.Put(20, "twenty")
// 	tree.Put(30, "thirty")
//
// 	// Floor 测试
// 	node, found := tree.Floor(25) // 应返回 20
// 	assert.True(t, found)
// 	assert.Equal(t, 20, node.Key)
//
// 	node, found = tree.Floor(10) // 应返回 10
// 	assert.True(t, found)
// 	assert.Equal(t, 10, node.Key)
//
// 	node, found = tree.Floor(5) // 不存在
// 	assert.False(t, found)
// 	assert.Nil(t, node)
//
// 	// Ceiling 测试
// 	node, found = tree.Ceiling(25) // 应返回 30
// 	assert.True(t, found)
// 	assert.Equal(t, 30, node.Key)
//
// 	node, found = tree.Ceiling(20) // 应返回 20
// 	assert.True(t, found)
// 	assert.Equal(t, 20, node.Key)
//
// 	node, found = tree.Ceiling(35) // 不存在
// 	assert.False(t, found)
// 	assert.Nil(t, node)
// }
//
// func TestRedBlackTree_Clear(t *testing.T) {
// 	tree, err := rbtree.New[int, string](intComparator)
// 	assert.NoError(t, err)
//
// 	tree.Put(1, "one")
// 	tree.Put(2, "two")
//
// 	assert.Equal(t, 2, tree.Size())
//
// 	tree.Clear()
//
// 	assert.True(t, tree.IsEmpty())
// 	assert.Equal(t, 0, tree.Size())
// }
//
// func TestRedBlackTree_KeysValues(t *testing.T) {
// 	tree, err := rbtree.New[int, string](intComparator)
// 	assert.NoError(t, err)
// 	tree.Put(3, "three")
// 	tree.Put(1, "one")
// 	tree.Put(2, "two")
//
// 	// Keys 应按排序顺序返回
// 	expectedKeys := []int{1, 2, 3}
// 	assert.Equal(t, expectedKeys, tree.Keys())
//
// 	// Values 应按 in-order 顺序返回
// 	expectedValues := []string{"one", "two", "three"}
// 	assert.Equal(t, expectedValues, tree.Values())
// }
//
// func TestRedBlackTree_Traversals(t *testing.T) {
// 	tree, err := rbtree.New[int, string](intComparator)
// 	assert.NoError(t, err)
// 	tree.Put(10, "ten")
// 	tree.Put(5, "five")
// 	tree.Put(15, "fifteen")
// 	tree.Put(3, "three")
// 	tree.Put(7, "seven")
//
// 	// Preorder: 根 → 左 → 右
// 	expectedPreorder := []int{10, 5, 3, 7, 15}
// 	var preorder []int
// 	for n := range tree.PreorderChan() {
// 		preorder = append(preorder, n.Key)
// 	}
// 	assert.Equal(t, expectedPreorder, preorder)
// 	preorder = make([]int, 0)
// 	tree.Preorder(func(i int, s string) {
// 		preorder = append(preorder, i)
// 	})
// 	assert.Equal(t, expectedPreorder, preorder)
//
// 	// Inorder: 左 → 根 → 右 (排序)
// 	expectedInorder := []int{3, 5, 7, 10, 15}
// 	var inorder []int
// 	for n := range tree.InorderChan() {
// 		inorder = append(inorder, n.Key)
// 	}
// 	assert.Equal(t, expectedInorder, inorder)
// 	inorder = make([]int, 0)
// 	tree.Inorder(func(i int, s string) {
// 		inorder = append(inorder, i)
// 	})
// 	assert.Equal(t, expectedInorder, inorder)
//
// 	// Postorder: 左 → 右 → 根
// 	expectedPostorder := []int{3, 7, 5, 15, 10}
// 	var postorder []int
// 	for n := range tree.PostorderChan() {
// 		postorder = append(postorder, n.Key)
// 	}
// 	assert.Equal(t, expectedPostorder, postorder)
// 	postorder = make([]int, 0)
// 	tree.Postorder(func(i int, s string) {
// 		postorder = append(postorder, i)
// 	})
// 	assert.Equal(t, expectedPostorder, postorder)
//
// 	// LevelOrder: 层级遍历
// 	expectedLevelOrder := []int{10, 5, 15, 3, 7}
// 	var levelOrder []int
// 	for n := range tree.LevelOrderChan() {
// 		levelOrder = append(levelOrder, n.Key)
// 	}
// 	assert.Equal(t, expectedLevelOrder, levelOrder)
// 	levelOrder = make([]int, 0)
// 	tree.LevelOrder(func(i int, s string) {
// 		levelOrder = append(levelOrder, i)
// 	})
// 	assert.Equal(t, expectedLevelOrder, levelOrder)
// }
//
// func TestRedBlackTree_String(t *testing.T) {
// 	fmt.Println("=== Test Red-Black Tree Visualization ===")
//
// 	// 1️⃣ 创建一个 int -> string 的红黑树
// 	tree, err := rbtree.NewWithOrderedKeys(rbtree.WithColorfulString[int, string]())
// 	assert.NoError(t, err)
// 	tree.Put(10, "ten")
// 	tree.Put(20, "twenty")
// 	tree.Put(30, "thirty")
// 	tree.Put(15, "fifteen")
// 	tree.Put(25, "twenty-five")
// 	tree.Put(5, "five")
// 	tree.Put(1, "one")
// 	tree.Put(7, "seven")
// 	tree.Put(40, "forty")
// 	tree.Put(50, "fifty")
//
// 	fmt.Println("\n🔹 Red-Black Tree (int -> string):")
// 	fmt.Println(tree.String())
//
// 	// 2️⃣ 创建一个 string -> int 的红黑树
// 	treeStr, err := rbtree.NewWithOrderedKeys(rbtree.WithColorfulString[string, int](), rbtree.WithNodeFormat[string, int]("%s:%d "))
// 	assert.NoError(t, err)
// 	treeStr.Put("banana", 10)
// 	treeStr.Put("apple", 5)
// 	treeStr.Put("cherry", 20)
// 	treeStr.Put("date", 15)
// 	treeStr.Put("fig", 25)
// 	treeStr.Put("grape", 8)
// 	treeStr.Put("lemon", 30)
//
// 	fmt.Println("\n🔹 Red-Black Tree (string -> int):")
// 	fmt.Println(treeStr.String())
//
// 	// 3️⃣ 创建一个 float64 -> string 的红黑树
// 	treeFloat, err := rbtree.NewWithOrderedKeys(rbtree.WithColorfulString[float64, string](), rbtree.WithNodeFormat[float64, string]("%.2f:%s "))
// 	assert.NoError(t, err)
//
// 	treeFloat.Put(3.14, "pi")
// 	treeFloat.Put(2.71, "e")
// 	treeFloat.Put(1.61, "golden ratio")
// 	treeFloat.Put(1.41, "sqrt(2)")
// 	treeFloat.Put(2.23, "sqrt(5)")
//
// 	fmt.Println("\n🔹 Red-Black Tree (float64 -> string):")
// 	fmt.Println(treeFloat.String())
//
// 	tt, _ := rbtree.NewWithOrderedKeys(rbtree.WithColorfulString[float64, float64]())
// 	r := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano())))
// 	for range 10000 {
// 		v := r.Float64()
// 		tt.Put(v, v)
// 	}
// 	fmt.Println(tt.Size(), tt.BlackCount(), tt.RedCount(), tt.LeafCount(), tt.MaxDepth(), tt.MinDepth())
// }

func newIntStringTree(t *testing.T) *rbtree.Tree[int, string] {
	t.Helper()
	tree, err := rbtree.NewOrderedKeys(rbtree.WithSafe[int, string]())
	require.NoError(t, err)
	return tree
}

func TestRedBlackTree_New(t *testing.T) {
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
			tree, err := rbtree.New[int, int](tt.cmp)
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

func TestRedBlackTree_Put(t *testing.T) {
	tree := newIntStringTree(t)

	// Test empty tree insert
	tree.Put(1, "one")
	assert.Equal(t, 1, tree.Size())
	assert.Equal(t, 1, tree.BlackCount(), "Root should be black")
	assert.Equal(t, 0, tree.RedCount())

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

	// Verify red-black properties
	assert.Positive(t, tree.BlackCount(), "Should have black nodes")
	// Additional color property checks can be added here
}

func TestRedBlackTree_Get(t *testing.T) {
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

func TestRedBlackTree_Delete(t *testing.T) {
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

	// Setup test tree
	tree.Put(4, "four")
	tree.Put(2, "two")
	tree.Put(6, "six")
	tree.Put(1, "one")
	tree.Put(3, "three")
	tree.Put(5, "five")
	tree.Put(7, "seven")

	initialBlackCount := tree.BlackCount()

	// Test delete leaf node
	val, exists = tree.Delete(7)
	assert.True(t, exists)
	assert.Equal(t, "seven", val)
	assert.Equal(t, 6, tree.Size())
	// Black count should remain same after deletion
	assert.Equal(t, initialBlackCount, tree.BlackCount())

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

func TestRedBlackTree_Clear(t *testing.T) {
	tree, err := rbtree.NewOrderedKeys[int, string]()
	require.NoError(t, err)

	tree.Put(1, "one")
	tree.Put(2, "two")

	assert.Equal(t, 2, tree.Size())

	tree.Clear()

	assert.True(t, tree.IsEmpty())
	assert.Equal(t, 0, tree.Size())
}

func TestRedBlackTree_KeysValues(t *testing.T) {
	tree, err := rbtree.NewOrderedKeys[int, string]()
	require.NoError(t, err)
	tree.Put(3, "three")
	tree.Put(1, "one")
	tree.Put(2, "two")

	// Keys 应按排序顺序返回
	expectedKeys := []int{1, 2, 3}
	assert.Equal(t, expectedKeys, tree.Keys())

	// Values 应按 in-order 顺序返回
	expectedValues := []string{"one", "two", "three"}
	assert.Equal(t, expectedValues, tree.Values())
}

func TestRedBlackTree_MinMax(t *testing.T) {
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

func TestRedBlackTree_FloorCeiling(t *testing.T) {
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

func TestRedBlackTree_RedBlackProperties(t *testing.T) {
	tree := newIntStringTree(t)

	// Insert some values
	values := []int{7, 3, 18, 10, 22, 8, 11, 26, 2, 6, 13}
	for _, v := range values {
		tree.Put(v, strconv.Itoa(v))
	}

	// Test black count
	blackCount := tree.BlackCount()
	assert.Positive(t, blackCount, "Should have black nodes")

	// Test red count
	redCount := tree.RedCount()
	assert.GreaterOrEqual(t, redCount, 0, "Red count should be non-negative")

	// Test leaf count
	leafCount := tree.LeafCount()
	assert.Positive(t, leafCount, "Should have leaf nodes")

	// Test max depth
	maxDepth := tree.MaxDepth()
	assert.Positive(t, maxDepth, "Should have positive depth")

	// Test min depth
	minDepth := tree.MinDepth()
	assert.Positive(t, minDepth, "Should have positive min depth")
	assert.LessOrEqual(t, minDepth, maxDepth, "Min depth should not exceed max depth")
}

func TestRedBlackTree_MarshalJSON(t *testing.T) {
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

func TestRedBlackTree_Traversals(t *testing.T) {
	tree, err := rbtree.NewOrderedKeys[int, string]()
	require.NoError(t, err)
	tree.Put(10, "ten")
	tree.Put(5, "five")
	tree.Put(15, "fifteen")
	tree.Put(3, "three")
	tree.Put(7, "seven")

	// Preorder: 根 → 左 → 右
	expectedPreorder := []int{10, 5, 3, 7, 15}
	preorder := make([]int, 0)
	tree.PreOrder(func(i int, s string) bool {
		preorder = append(preorder, i)
		return true
	})
	assert.Equal(t, expectedPreorder, preorder)

	// Inorder: 左 → 根 → 右 (排序)
	expectedInorder := []int{3, 5, 7, 10, 15}
	inorder := make([]int, 0)
	tree.InOrder(func(i int, s string) bool {
		inorder = append(inorder, i)
		return true
	})
	assert.Equal(t, expectedInorder, inorder)

	// Postorder: 左 → 右 → 根
	expectedPostorder := []int{3, 7, 5, 15, 10}
	postorder := make([]int, 0)
	tree.PostOrder(func(i int, s string) bool {
		postorder = append(postorder, i)
		return true
	})
	assert.Equal(t, expectedPostorder, postorder)

	// LevelOrder: 层级遍历
	expectedLevelOrder := []int{10, 5, 15, 3, 7}
	levelOrder := make([]int, 0)
	tree.LevelOrder(func(i int, s string) bool {
		levelOrder = append(levelOrder, i)
		return true
	})
	assert.Equal(t, expectedLevelOrder, levelOrder)

	tree = newIntStringTree(t)
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
	result := make([]int, 0)
	tree.InOrder(func(key int, value string) bool {
		result = append(result, key)
		return key < 5
	})
	expected := []int{1, 2, 3, 4, 5}
	assert.Equal(t, expected, result)
}

func TestRedBlackTree_String(t *testing.T) {
	fmt.Println("=== Test Red-Black Tree Visualization ===")

	// 1️⃣ 创建一个 int -> string 的红黑树
	tree, err := rbtree.NewOrderedKeys(rbtree.WithColorfulString[int, string]())
	require.NoError(t, err)
	tree.Put(10, "ten")
	tree.Put(20, "twenty")
	tree.Put(30, "thirty")
	tree.Put(15, "fifteen")
	tree.Put(25, "twenty-five")
	tree.Put(5, "five")
	tree.Put(1, "one")
	tree.Put(7, "seven")
	tree.Put(40, "forty")
	tree.Put(50, "fifty")

	fmt.Println("\n🔹 Red-Black Tree (int -> string):")
	fmt.Println(tree.String())

	// 2️⃣ 创建一个 string -> int 的红黑树
	treeStr, err := rbtree.NewOrderedKeys(rbtree.WithColorfulString[string, int](), rbtree.WithNodeFormatter(func(k string, v int) string {
		return fmt.Sprintf("%s:%d ", k, v)
	}))
	require.NoError(t, err)
	treeStr.Put("banana", 10)
	treeStr.Put("apple", 5)
	treeStr.Put("cherry", 20)
	treeStr.Put("date", 15)
	treeStr.Put("fig", 25)
	treeStr.Put("grape", 8)
	treeStr.Put("lemon", 30)

	fmt.Println("\n🔹 Red-Black Tree (string -> int):")
	fmt.Println(treeStr.String())

	// 3️⃣ 创建一个 float64 -> string 的红黑树
	treeFloat, err := rbtree.NewOrderedKeys(rbtree.WithColorfulString[float64, string](), rbtree.WithNodeFormatter(func(k float64, v string) string {
		return fmt.Sprintf("%.2f:%s ", k, v)
	}))
	require.NoError(t, err)

	treeFloat.Put(3.14, "pi")
	treeFloat.Put(2.71, "e")
	treeFloat.Put(1.61, "golden ratio")
	treeFloat.Put(1.41, "sqrt(2)")
	treeFloat.Put(2.23, "sqrt(5)")

	fmt.Println("\n🔹 Red-Black Tree (float64 -> string):")
	fmt.Println(treeFloat.String())

	tt, _ := rbtree.NewOrderedKeys(rbtree.WithColorfulString[float64, float64]())
	r := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano())))
	for range 10000 {
		v := r.Float64()
		tt.Put(v, v)
	}
	fmt.Println(tt.Size(), tt.BlackCount(), tt.RedCount(), tt.LeafCount(), tt.MaxDepth(), tt.MinDepth())
}
