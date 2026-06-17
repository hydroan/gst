package mapset_test

import (
	"testing"

	"github.com/hydroan/gst/ds/mapset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intCmp(a, b int) int {
	return a - b
}

func stringCmp(a, b string) int {
	if a > b {
		return 1
	} else if a < b {
		return -1
	}
	return 0
}

func TestNew(t *testing.T) {
	s, err := mapset.New[int]()
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.True(t, s.IsEmpty())
}

func TestNewFromSlice(t *testing.T) {
	s, err := mapset.NewFromSlice([]int{1, 2, 3, 4}, mapset.WithSorted(intCmp))
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, 4, s.Len())
	assert.Equal(t, []int{1, 2, 3, 4}, s.Slice())
}

func TestNewFromMapKeys(t *testing.T) {
	m := map[int]string{1: "a", 2: "b", 3: "c"}
	s, err := mapset.NewFromMapKeys(m, mapset.WithSorted(intCmp))
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, 3, s.Len())
	assert.Equal(t, []int{1, 2, 3}, s.Slice())
}

func TestNewFromMapValues(t *testing.T) {
	m := map[int]string{1: "a", 2: "b", 3: "c"}
	s, err := mapset.NewFromMapValues(m, mapset.WithSorted(stringCmp))
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, 3, s.Len())
	assert.Equal(t, []string{"a", "b", "c"}, s.Slice())
}

func TestAdd(t *testing.T) {
	s, _ := mapset.New(mapset.WithSorted(intCmp))
	added := s.Add(1, 2, 3)
	assert.Equal(t, 3, added)
	assert.Equal(t, 3, s.Len())
	assert.Equal(t, []int{1, 2, 3}, s.Slice())
}

func TestRemove(t *testing.T) {
	s, _ := mapset.New[int](mapset.WithSorted(intCmp))
	s.Add(1, 2, 3)
	assert.True(t, s.ContainsOne(2))
	assert.True(t, s.Contains(2))
	assert.Equal(t, []int{1, 2, 3}, s.Slice())
	s.Remove(2)
	assert.Equal(t, 2, s.Len())
	assert.False(t, s.ContainsOne(2))
	assert.False(t, s.Contains(2))
	assert.Equal(t, []int{1, 3}, s.Slice())
}

func TestPop(t *testing.T) {
	s, _ := mapset.New[int]()
	s.Add(1, 2, 3)
	elem, ok := s.Pop()
	assert.True(t, ok)
	assert.Contains(t, []int{1, 2, 3}, elem)
	assert.Equal(t, 2, s.Len())
}

func TestClear(t *testing.T) {
	s, _ := mapset.New[int]()
	s.Add(1, 2, 3)
	s.Clear()
	assert.True(t, s.IsEmpty())
	assert.Equal(t, 0, s.Len())
}

func TestContains(t *testing.T) {
	s, _ := mapset.New[int]()
	s.Add(1, 2, 3)
	assert.True(t, s.Contains(1))
	assert.True(t, s.Contains(1, 2))
	assert.True(t, s.Contains(1, 2, 3))
	assert.False(t, s.Contains(1, 2, 3, 4))
	assert.False(t, s.Contains(4))
}

func TestContainsOne(t *testing.T) {
	s, _ := mapset.New[int]()
	s.Add(1, 2, 3)
	assert.True(t, s.ContainsOne(1))
	assert.True(t, s.ContainsOne(2))
	assert.True(t, s.ContainsOne(3))
	assert.False(t, s.ContainsOne(4))
}

func TestContainsAny(t *testing.T) {
	s, _ := mapset.New[int]()
	s.Add(1, 2, 3)
	assert.True(t, s.ContainsAny(1))
	assert.True(t, s.ContainsAny(1, 2))
	assert.True(t, s.ContainsAny(1, 2, 3))
	assert.True(t, s.ContainsAny(1, 2, 3, 4))
	assert.False(t, s.Contains(4))
}

func TestEqual(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s2, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s3, _ := mapset.NewFromSlice([]int{4, 5, 6})
	assert.True(t, s1.Equal(s2))
	assert.False(t, s1.Equal(s3))
}

func TestRange(t *testing.T) {
	s, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSorted(intCmp))

	elements := make([]int, 0)
	count := 0
	s.Range(func(e int) bool {
		elements = append(elements, e)
		count++
		return true
	})

	assert.Equal(t, 3, count)
	assert.Equal(t, []int{1, 2, 3}, elements)
}

func TestIsEmpty(t *testing.T) {
	s, _ := mapset.New[int]()
	assert.True(t, s.IsEmpty())
	s.Add(1)
	assert.False(t, s.IsEmpty())
}

func TestIter(t *testing.T) {
	s, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSorted(intCmp))
	elements := make([]int, 0, 3)
	count := 0
	for e := range s.Iter() {
		elements = append(elements, e)
		count++
	}
	assert.Equal(t, 3, count)
	assert.Equal(t, []int{1, 2, 3}, elements)
}

func TestIsSubset(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2})
	s2, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s3, _ := mapset.NewFromSlice([]int{1, 2, 3})
	assert.True(t, s1.IsSubset(s2))
	assert.True(t, s2.IsSubset(s3))
	assert.False(t, s2.IsSubset(s1))
}

func TestIsProperSubset(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2})
	s2, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s3, _ := mapset.NewFromSlice([]int{1, 2, 3})
	assert.True(t, s1.IsProperSubset(s2))
	assert.False(t, s2.IsProperSubset(s3))
	assert.False(t, s2.IsProperSubset(s1))
}

func TestIsSuperset(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s2, _ := mapset.NewFromSlice([]int{1, 2})
	s3, _ := mapset.NewFromSlice([]int{1, 2})
	assert.True(t, s1.IsSuperset(s2))
	assert.True(t, s2.IsSuperset(s3))
	assert.False(t, s2.IsSuperset(s1))
}

func TestIsProperSuperset(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s2, _ := mapset.NewFromSlice([]int{1, 2})
	s3, _ := mapset.NewFromSlice([]int{1, 2})
	assert.True(t, s1.IsProperSuperset(s2))
	assert.False(t, s2.IsProperSuperset(s3))
	assert.False(t, s2.IsProperSuperset(s1))
}

func TestDifference(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSorted(intCmp))
	s2, _ := mapset.NewFromSlice([]int{2, 3, 4}, mapset.WithSorted(intCmp))
	diff1 := s1.Difference(s2)
	diff2 := s2.Difference(s1)
	assert.Equal(t, []int{1}, diff1.Slice())
	assert.Equal(t, []int{4}, diff2.Slice())
}

func TestSymmetricDifference(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s2, _ := mapset.NewFromSlice([]int{2, 3, 4})
	diff1 := s1.SymmetricDifference(s2)
	diff2 := s2.SymmetricDifference(s1)
	// fmt.Println(diff1.Slice(), diff2.Slice())
	assert.True(t, diff1.Contains(1, 4))
	assert.True(t, diff2.Contains(1, 4))
}

func TestUnion(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s2, _ := mapset.NewFromSlice([]int{3, 4, 5})
	union := s1.Union(s2)
	assert.Equal(t, 5, union.Len())
	assert.True(t, union.Contains(1, 2, 3, 4, 5))
}

func TestIntersect(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s2, _ := mapset.NewFromSlice([]int{2, 3, 4})
	intersect := s1.Intersect(s2)
	assert.Equal(t, 2, intersect.Len())
	assert.True(t, intersect.Contains(2, 3))
}

func TestClone(t *testing.T) {
	s1, _ := mapset.NewFromSlice([]int{1, 2, 3})
	s2 := s1.Clone()
	assert.True(t, s1.Equal(s2))
	s1.Add(4)
	assert.False(t, s1.Equal(s2))
	s2.Add(4)
	assert.True(t, s1.Equal(s2))
}

func TestSlice(t *testing.T) {
	s, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSorted(intCmp))
	assert.Equal(t, []int{1, 2, 3}, s.Slice())
}

func TestMarshalJSON(t *testing.T) {
	s, _ := mapset.NewFromSlice([]int{1, 2, 3}, mapset.WithSorted(intCmp))
	data, err := s.MarshalJSON()
	require.NoError(t, err)
	assert.JSONEq(t, "[1,2,3]", string(data))
}

func TestUnmarshalJSON(t *testing.T) {
	data := []byte("[1,2,3]")
	var s mapset.Set[int]
	err := s.UnmarshalJSON(data)
	require.NoError(t, err)
	assert.True(t, s.Contains(1, 2, 3))
}
