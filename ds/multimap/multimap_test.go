package multimap_test

import (
	"testing"

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
			wantErr bool
		}{
			{
				name:    "with valid equal function",
				cmp:     intCmp,
				wantErr: false,
			},
			{
				name:    "with nil equal function",
				cmp:     nil,
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mm, err := multimap.New[string, int](tt.cmp)
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
	mm.Set("a", 1)
	mm.Set("a", 2)

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

	t.Run("Set", func(t *testing.T) {
		mm.Set("a", 1)
		mm.Set("a", 2)

		vals, exists := mm.Get("a")
		assert.True(t, exists)
		assert.Equal(t, []int{1, 2}, vals)
	})

	t.Run("SetAll", func(t *testing.T) {
		original := []int{3, 4}
		mm.SetAll("b", original)

		// Modify original to verify deep copy
		original[0] = 100

		vals, exists := mm.Get("b")
		assert.True(t, exists)
		assert.Equal(t, []int{3, 4}, vals)
	})
}

func TestMultiMap_DeleteOperations(t *testing.T) {
	mm, _ := multimap.New[string, int](intCmp)
	mm.Set("a", 1)
	mm.Set("a", 2)
	mm.Set("a", 1)
	mm.Set("b", 3)

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
	mm.Set("a", 1)
	mm.Set("a", 2)
	mm.Set("b", 3)

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
		mm.Set("a", 1)
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
	original.Set("a", 1)
	original.Set("a", 2)

	t.Run("Clone", func(t *testing.T) {
		cloned := original.Clone()

		// Modify original
		original.Set("a", 3)

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
