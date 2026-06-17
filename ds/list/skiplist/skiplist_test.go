package skiplist_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/ds/list/skiplist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intCmp(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func TestSkipList_New(t *testing.T) {
	t.Run("valid creation", func(t *testing.T) {
		sl, err := skiplist.New[int, string](intCmp)
		require.NoError(t, err)
		assert.NotNil(t, sl)
		assert.True(t, sl.IsEmpty())
		assert.Equal(t, 0, sl.Len())
	})

	t.Run("nil comparison function", func(t *testing.T) {
		sl, err := skiplist.New[int, string](nil)
		require.Error(t, err)
		assert.Nil(t, sl)
	})

	t.Run("from map", func(t *testing.T) {
		m := map[int]string{
			1: "one",
			2: "two",
			3: "three",
		}
		expectedKeys := []int{1, 2, 3}
		expectedValues := []string{"one", "two", "three"}
		sl, err := skiplist.NewFromOrderedMap(m)
		require.NoError(t, err)
		assert.NotNil(t, sl)
		assert.Equal(t, expectedKeys, sl.Keys())
		assert.Equal(t, expectedValues, sl.Values())
	})
}

func TestSkipList_BasicOperations(t *testing.T) {
	sl, _ := skiplist.New[int, string](intCmp)

	t.Run("put and get", func(t *testing.T) {
		sl.Put(1, "one")
		sl.Put(2, "two")
		sl.Put(3, "three")

		v, found := sl.Get(2)
		assert.True(t, found)
		assert.Equal(t, "two", v)

		v, found = sl.Get(4)
		assert.False(t, found)
		assert.Empty(t, v)
	})

	t.Run("update existing key", func(t *testing.T) {
		sl.Put(2, "TWO")
		v, found := sl.Get(2)
		assert.True(t, found)
		assert.Equal(t, "TWO", v)
	})

	t.Run("delete", func(t *testing.T) {
		v, found := sl.Delete(2)
		assert.True(t, found)
		assert.Equal(t, "TWO", v)

		v, found = sl.Get(2)
		assert.False(t, found)
		assert.Empty(t, v)
	})

	t.Run("delete non-existent", func(t *testing.T) {
		v, found := sl.Delete(99)
		assert.False(t, found)
		assert.Empty(t, v)
	})
}

func TestSkipList_MinMax(t *testing.T) {
	sl, _ := skiplist.New[int, string](intCmp)

	t.Run("empty skiplist", func(t *testing.T) {
		k, v, found := sl.Min()
		assert.False(t, found)
		assert.Empty(t, k)
		assert.Empty(t, v)

		k, v, found = sl.Max()
		assert.False(t, found)
		assert.Empty(t, k)
		assert.Empty(t, v)
	})

	t.Run("single element", func(t *testing.T) {
		sl.Put(1, "one")

		k, v, found := sl.Min()
		assert.True(t, found)
		assert.Equal(t, 1, k)
		assert.Equal(t, "one", v)

		k, v, found = sl.Max()
		assert.True(t, found)
		assert.Equal(t, 1, k)
		assert.Equal(t, "one", v)
	})

	t.Run("multiple elements", func(t *testing.T) {
		sl.Put(5, "five")
		sl.Put(3, "three")
		sl.Put(7, "seven")

		k, v, found := sl.Min()
		assert.True(t, found)
		assert.Equal(t, 1, k)
		assert.Equal(t, "one", v)

		k, v, found = sl.Max()
		assert.True(t, found)
		assert.Equal(t, 7, k)
		assert.Equal(t, "seven", v)
	})
}

func TestSkipList_FloorCeiling(t *testing.T) {
	sl, _ := skiplist.New[int, string](intCmp)

	t.Run("empty skiplist", func(t *testing.T) {
		k, v, found := sl.Floor(5)
		assert.False(t, found)
		assert.Empty(t, k)
		assert.Empty(t, v)

		k, v, found = sl.Ceiling(5)
		assert.False(t, found)
		assert.Empty(t, k)
		assert.Empty(t, v)
	})

	// Setup test data
	sl.Put(10, "ten")
	sl.Put(20, "twenty")
	sl.Put(30, "thirty")
	sl.Put(40, "forty")

	t.Run("exact match", func(t *testing.T) {
		k, v, found := sl.Floor(20)
		assert.True(t, found)
		assert.Equal(t, 20, k)
		assert.Equal(t, "twenty", v)

		k, v, found = sl.Ceiling(20)
		assert.True(t, found)
		assert.Equal(t, 20, k)
		assert.Equal(t, "twenty", v)
	})

	t.Run("between elements", func(t *testing.T) {
		k, v, found := sl.Floor(25)
		assert.True(t, found)
		assert.Equal(t, 20, k)
		assert.Equal(t, "twenty", v)

		k, v, found = sl.Ceiling(25)
		assert.True(t, found)
		assert.Equal(t, 30, k)
		assert.Equal(t, "thirty", v)
	})

	t.Run("beyond bounds", func(t *testing.T) {
		k, v, found := sl.Floor(5)
		assert.False(t, found)
		assert.Empty(t, k)
		assert.Empty(t, v)

		k, v, found = sl.Ceiling(45)
		assert.False(t, found)
		assert.Empty(t, k)
		assert.Empty(t, v)
	})
}

func TestSkipList_KeysValues(t *testing.T) {
	sl, _ := skiplist.New[int, string](intCmp)

	t.Run("empty skiplist", func(t *testing.T) {
		assert.Empty(t, sl.Keys())
		assert.Empty(t, sl.Values())
	})

	t.Run("single element", func(t *testing.T) {
		sl.Put(1, "one")

		assert.Equal(t, []int{1}, sl.Keys())
		assert.Equal(t, []string{"one"}, sl.Values())
	})

	t.Run("multiple elements", func(t *testing.T) {
		sl.Clear()
		sl.Put(3, "three")
		sl.Put(1, "one")
		sl.Put(4, "four")
		sl.Put(2, "two")

		assert.Equal(t, []int{1, 2, 3, 4}, sl.Keys())
		assert.Equal(t, []string{"one", "two", "three", "four"}, sl.Values())
	})
}

func TestSkipList_Range(t *testing.T) {
	sl, _ := skiplist.New[int, string](intCmp)

	t.Run("empty skiplist", func(t *testing.T) {
		count := 0
		sl.Range(func(k int, v string) bool {
			count++
			return true
		})
		assert.Equal(t, 0, count)
	})

	t.Run("nil callback", func(t *testing.T) {
		sl.Put(1, "one")
		// Should not panic
		sl.Range(nil)
	})

	t.Run("single element", func(t *testing.T) {
		sl.Clear()
		sl.Put(1, "one")

		var keys []int
		var values []string
		sl.Range(func(k int, v string) bool {
			keys = append(keys, k)
			values = append(values, v)
			return true
		})
		assert.Equal(t, []int{1}, keys)
		assert.Equal(t, []string{"one"}, values)
	})

	t.Run("multiple elements", func(t *testing.T) {
		sl.Clear()
		elements := map[int]string{
			3: "three",
			1: "one",
			4: "four",
			2: "two",
		}
		for k, v := range elements {
			sl.Put(k, v)
		}

		var keys []int
		var values []string
		sl.Range(func(k int, v string) bool {
			keys = append(keys, k)
			values = append(values, v)
			return true
		})
		// Verify order is ascending
		assert.Equal(t, []int{1, 2, 3, 4}, keys)
		assert.Equal(t, []string{"one", "two", "three", "four"}, values)
	})

	t.Run("early termination", func(t *testing.T) {
		sl.Clear()
		for i := 1; i <= 5; i++ {
			sl.Put(i, fmt.Sprintf("value%d", i))
		}

		var keys []int
		sl.Range(func(k int, v string) bool {
			keys = append(keys, k)
			return k < 3 // Stop after collecting keys 1 and 2
		})
		assert.Equal(t, []int{1, 2, 3}, keys)
	})

	t.Run("concurrent safety", func(t *testing.T) {
		sl, _ := skiplist.New[int, string](intCmp, skiplist.WithSafe[int, string]())
		for i := 1; i <= 3; i++ {
			sl.Put(i, fmt.Sprintf("value%d", i))
		}

		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine to range over elements
		go func() {
			defer wg.Done()
			sl.Range(func(k int, v string) bool {
				time.Sleep(time.Millisecond) // Simulate work
				return true
			})
		}()

		// Goroutine to add new elements
		go func() {
			defer wg.Done()
			sl.Put(4, "value4")
			sl.Put(5, "value5")
		}()

		wg.Wait() // Should not deadlock
	})
}

func TestSkipList_Clear(t *testing.T) {
	sl, _ := skiplist.New[int, string](intCmp)

	t.Run("clear empty list", func(t *testing.T) {
		sl.Clear()
		assert.True(t, sl.IsEmpty())
		assert.Equal(t, 0, sl.Len())
	})

	t.Run("clear populated list", func(t *testing.T) {
		sl.Put(1, "one")
		sl.Put(2, "two")
		sl.Put(3, "three")
		assert.Equal(t, 3, sl.Len())

		sl.Clear()
		assert.True(t, sl.IsEmpty())
		assert.Equal(t, 0, sl.Len())
		_, found := sl.Get(1)
		assert.False(t, found)
	})
}

func TestSkipList_Clone(t *testing.T) {
	sl, _ := skiplist.New[int, string](intCmp)

	// t.Run("clone empty list", func(t *testing.T) {
	// 	clone := sl.Clone()
	// 	assert.Equal(t, sl.Len(), clone.Len())
	// 	assert.True(t, clone.IsEmpty())
	// })

	t.Run("clone populated list", func(t *testing.T) {
		sl.Put(1, "one")
		sl.Put(2, "two")
		sl.Put(3, "three")

		clone := sl.Clone()
		assert.Equal(t, sl.Len(), clone.Len())
		assert.Equal(t, sl.Keys(), clone.Keys())
		assert.Equal(t, sl.Values(), clone.Values())
	})

	t.Run("independence of clones", func(t *testing.T) {
		original := sl.Clone()
		// Modify original
		sl.Put(4, "four")
		assert.NotEqual(t, original.Len(), sl.Len())
		_, found := original.Get(4)
		assert.False(t, found)

		// Modify clone
		clone := sl.Clone()
		clone.Delete(1)
		v, found := sl.Get(1)
		assert.True(t, found)
		assert.Equal(t, "one", v)
	})

	t.Run("clone maintains order", func(t *testing.T) {
		sl.Clear()
		sl.Put(3, "three")
		sl.Put(1, "one")
		sl.Put(4, "four")
		sl.Put(2, "two")

		clone := sl.Clone()
		assert.Equal(t, sl.Keys(), clone.Keys())
		assert.Equal(t, sl.Values(), clone.Values())
	})
}

func TestSkipList_Encoding(t *testing.T) {
	m := map[int]string{
		1: "one",
		2: "two",
		3: "three",
	}
	sl, _ := skiplist.NewFromOrderedMap(m)
	b, err := json.Marshal(sl)
	if err != nil {
		t.Fatal(t, err)
	}

	sl2, _ := skiplist.NewOrdered[int, string]()
	err = json.Unmarshal(b, sl2)
	if err != nil {
		t.Fatal(t, err)
	}
	assert.Equal(t, sl.Keys(), sl2.Keys())
	assert.Equal(t, sl.Values(), sl2.Values())

	sl.Delete(1)
	assert.NotEqual(t, sl.Keys(), sl2.Keys())
	assert.NotEqual(t, sl.Values(), sl2.Values())
}

func TestSkipList_String(t *testing.T) {
	m := map[int]string{
		10: "ten",
		30: "thirty",
		20: "twenty",
		15: "fifteen",
		25: "twenty-five",
		5:  "five",
		35: "thirty-five",
		40: "forty",
		50: "fifty",
		1:  "one",
		2:  "two",
		3:  "three",
		4:  "four",
		6:  "six",
		7:  "seven",
		8:  "eight",
		9:  "nine",
	}

	sl, _ := skiplist.NewFromOrderedMap(m, skiplist.WithNodeFormatter(func(k int, v string) string { return fmt.Sprintf("%d:%s", k, v) }))
	fmt.Println(sl)
}
