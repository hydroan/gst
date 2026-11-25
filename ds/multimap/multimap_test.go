package multimap_test

import (
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/ds/container"
	"github.com/hydroan/gst/ds/multimap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intCmp(a, b int) int {
	return a - b
}

func TestMultiMap_Creation(t *testing.T) {
	t.Run("New", func(t *testing.T) {
		tests := []struct {
			name    string
			cmp     func(int, int) int
			wantErr error
		}{
			{
				name: "with valid comparison function",
				cmp:  intCmp,
			},
			{
				name:    "with nil comparison function",
				cmp:     nil,
				wantErr: container.ErrComparisonNil,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mm, err := multimap.New[string, int](tt.cmp)
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
					assert.Nil(t, mm)
				} else {
					require.NoError(t, err)
					assert.NotNil(t, mm)
				}
			})
		}
	})

	t.Run("NewFromMap", func(t *testing.T) {
		tests := []struct {
			name    string
			input   map[string][]int
			cmp     func(int, int) int
			wantErr bool
		}{
			{
				name:    "with valid map and equal",
				input:   map[string][]int{"a": {1, 2}},
				cmp:     intCmp,
				wantErr: false,
			},
			{
				name:    "with nil map",
				input:   nil,
				cmp:     intCmp,
				wantErr: false,
			},
			{
				name:    "with nil equal",
				input:   map[string][]int{"a": {1, 2}},
				cmp:     nil,
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mm, err := multimap.NewFromMap(tt.input, tt.cmp)
				if tt.wantErr {
					require.Error(t, err)
					assert.Nil(t, mm)
				} else {
					require.NoError(t, err)
					assert.NotNil(t, mm)
				}
			})
		}
	})
}

func TestMultiMap_GetOperations(t *testing.T) {
	mm, _ := multimap.New[string, int](intCmp)
	mm.Add("a", 1)
	mm.Add("a", 2)

	t.Run("Get", func(t *testing.T) {
		tests := []struct {
			name      string
			key       string
			wantVals  []int
			wantExist bool
		}{
			{
				name:      "existing key",
				key:       "a",
				wantVals:  []int{1, 2},
				wantExist: true,
			},
			{
				name:      "non-existing key",
				key:       "b",
				wantVals:  nil,
				wantExist: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				vals, exists := mm.Get(tt.key)
				assert.Equal(t, tt.wantExist, exists)
				assert.Equal(t, tt.wantVals, vals)
			})
		}
	})

	t.Run("GetOne", func(t *testing.T) {
		val, exists := mm.GetOne("a")
		assert.True(t, exists)
		assert.Equal(t, 1, val)

		val, exists = mm.GetOne("b")
		assert.False(t, exists)
		assert.Zero(t, val)
	})
}

func TestMultiMap_SetOperations(t *testing.T) {
	mm, _ := multimap.New[string, int](intCmp)

	t.Run("Add appends a value", func(t *testing.T) {
		mm.Add("a", 1)
		mm.Add("a", 2)

		vals, exists := mm.Get("a")
		assert.True(t, exists)
		assert.Equal(t, []int{1, 2}, vals)
	})

	t.Run("Set replaces the values with a copy", func(t *testing.T) {
		mm.Add("b", 9)
		original := []int{3, 4}
		mm.Set("b", original...)

		// Modify original to verify deep copy
		original[0] = 100

		vals, exists := mm.Get("b")
		assert.True(t, exists)
		assert.Equal(t, []int{3, 4}, vals)
	})

	t.Run("Set with no values removes the key", func(t *testing.T) {
		mm.Add("c", 1)
		mm.Set("c")

		assert.False(t, mm.Has("c"))
		assert.Zero(t, mm.Count("c"))
	})
}

func TestMultiMap_DeleteOperations(t *testing.T) {
	mm, _ := multimap.New[string, int](intCmp)
	mm.Add("a", 1)
	mm.Add("a", 2)
	mm.Add("a", 1)
	mm.Add("b", 3)

	t.Run("Delete", func(t *testing.T) {
		mm.Delete("a")
		assert.False(t, mm.Has("a"))

		// Delete non-existent key should not panic
		mm.Delete("non-existent")
	})

	t.Run("DeleteValue", func(t *testing.T) {
		count := mm.DeleteValue("b", 3)
		assert.Equal(t, 1, count)
		assert.False(t, mm.Has("b"))

		// Delete non-existent value
		count = mm.DeleteValue("b", 999)
		assert.Equal(t, 0, count)
	})
}

func TestMultiMap_QueryOperations(t *testing.T) {
	mm, _ := multimap.New[string, int](intCmp)
	mm.Add("a", 1)
	mm.Add("a", 2)
	mm.Add("b", 3)

	t.Run("Size and Length", func(t *testing.T) {
		assert.Equal(t, 2, mm.Len())
		assert.Equal(t, 3, mm.Size())
	})

	t.Run("IsEmpty", func(t *testing.T) {
		assert.False(t, mm.IsEmpty())
		mm.Clear()
		assert.True(t, mm.IsEmpty())
	})

	t.Run("Has and Contains", func(t *testing.T) {
		mm.Add("a", 1)
		assert.True(t, mm.Has("a"))
		assert.True(t, mm.Contains("a", 1))
		assert.False(t, mm.Contains("a", 999))
	})

	t.Run("Keys and Values", func(t *testing.T) {
		keys := mm.Keys()
		assert.Len(t, keys, 1)
		assert.Contains(t, keys, "a")

		values := mm.Values()
		assert.Len(t, values, 1)
		assert.Contains(t, values, 1)
	})
}

func TestMultiMap_CloneAndMap(t *testing.T) {
	original, _ := multimap.New[string, int](intCmp)
	original.Add("a", 1)
	original.Add("a", 2)

	t.Run("Clone", func(t *testing.T) {
		cloned := original.Clone()

		// Modify original
		original.Add("a", 3)

		// Verify cloned remains unchanged
		vals, exists := cloned.Get("a")
		assert.True(t, exists)
		assert.Equal(t, []int{1, 2}, vals)
	})

	t.Run("Map", func(t *testing.T) {
		m := original.Map()

		// Modify map
		m["a"] = append(m["a"], 4)

		// Verify original remains unchanged
		vals, exists := original.Get("a")
		assert.True(t, exists)
		assert.Equal(t, []int{1, 2, 3}, vals)
	})
}

// TestMultiMap_CloneHasItsOwnLock pins that a clone of a map made safe for
// concurrent use locks on its own: writing to the clone while the original is
// being ranged over, and so holds its read lock, does not wait on it.
func TestMultiMap_CloneHasItsOwnLock(t *testing.T) {
	original, err := multimap.New[string, int](intCmp, multimap.WithSafe[string, int]())
	require.NoError(t, err)
	original.Add("a", 1)
	cloned := original.Clone()

	requireFinishes(t, func() {
		original.Range(func(key string, _ []int) bool {
			cloned.Add(key, 2)
			return true
		})
	})
	vals, _ := cloned.Get("a")
	assert.Equal(t, []int{1, 2}, vals)
	vals, _ = original.Get("a")
	assert.Equal(t, []int{1}, vals)
}

// TestMultiMap_ConcurrentUse pins what WithSafe gives: the methods of a map
// called from several goroutines at once lose nothing, and leave no race for
// the race detector to report.
func TestMultiMap_ConcurrentUse(t *testing.T) {
	m, err := multimap.New[int, int](intCmp, multimap.WithSafe[int, int]())
	require.NoError(t, err)

	const workers, rounds = 8, 100
	var wg sync.WaitGroup
	for w := range workers {
		// Each writer keeps to keys of its own, so what it leaves is known:
		// every value under the first, nothing under the other two.
		wg.Go(func() {
			for i := range rounds {
				m.Add(w, i)
				m.Set(w+workers, i)
				m.DeleteValue(w+workers, i)
				m.Add(w+2*workers, i)
				m.Delete(w + 2*workers)
			}
		})
		wg.Go(func() {
			for range rounds {
				_, _ = m.Get(w)
				_, _ = m.GetOne(w)
				_ = m.Has(w)
				_ = m.Contains(w, 0)
				_ = m.Count(w)
				_ = m.Len()
				_ = m.Size()
				_ = m.IsEmpty()
				_ = m.Keys()
				_ = m.Values()
				_ = m.Map()
				_ = m.Clone()
				m.Range(func(int, []int) bool { return true })
			}
		})
	}
	wg.Wait()

	assert.Equal(t, workers, m.Len())
	assert.Equal(t, workers*rounds, m.Size())
	m.Clear()
	assert.True(t, m.IsEmpty())
}

// requireFinishes fails the test when fn has not returned within ten seconds,
// which is how a deadlock shows up.
func requireFinishes(t *testing.T, fn func()) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("fn did not finish: it deadlocked")
	}
}
