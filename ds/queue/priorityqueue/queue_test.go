package priorityqueue_test

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	pq "github.com/hydroan/gst/ds/queue/priorityqueue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type User struct {
	Name string
	ID   int
}

func (u *User) String() string {
	if u == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", u.Name, u.ID)
}

type Users []*User

func (ul Users) Len() int           { return len(ul) }
func (ul Users) Less(i, j int) bool { return ul[i].ID < ul[j].ID }
func (ul Users) Swap(i, j int)      { ul[i], ul[j] = ul[j], ul[i] }

func userCmp(u1, u2 *User) int {
	if u1 == nil && u2 == nil {
		return 0
	}
	if u1 == nil {
		return -1
	}
	if u2 == nil {
		return 1
	}
	return u1.ID - u2.ID
}

var users = []*User{
	{"user03", 3},
	{"user02", 2},
	{"user01", 1},
	{"user04", 4},
	{"user05", 5},
	{"user05", 5},
	{"user03", 3},
	{"user06", 6},
	{"user09", 9},
	{"user08", 8},
	{"user0", 0},
	{"user10", 10},
}

func TestPriorityQueue_CustomStruct(t *testing.T) {
	q, err := pq.New(userCmp)
	require.NoError(t, err)
	for _, u := range users {
		q.Enqueue(u)
	}
	// fmt.Println(q.Values())
	sorted1 := make([]*User, len(users))
	sorted2 := make([]*User, len(users))
	copy(sorted1, users)
	copy(sorted2, users)
	sort.Sort(Users(sorted1))
	sort.Sort(sort.Reverse(Users(sorted2)))
	// fmt.Println(sorted1)
	// fmt.Println(sorted2)
	// fmt.Println(users)
	assert.Equal(t, sorted1, q.Values())

	q, err = pq.New(userCmp, pq.WithMaxPriority[*User]())
	require.NoError(t, err)
	for _, u := range users {
		q.Enqueue(u)
	}
	assert.Equal(t, sorted2, q.Values())
	// fmt.Println(q.Values())
}

func intCmp(a, b int) int {
	return a - b
}

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		cmp       func(int, int) int
		opts      []pq.Option[int]
		wantError bool
	}{
		{
			name:      "nil comparison function",
			cmp:       nil,
			wantError: true,
		},
		{
			name:      "valid comparison function",
			cmp:       intCmp,
			wantError: false,
		},
		{
			name: "with safe option",
			cmp:  intCmp,
			opts: []pq.Option[int]{pq.WithSafe[int]()},
		},
		{
			name: "with max priority option",
			cmp:  intCmp,
			opts: []pq.Option[int]{pq.WithMaxPriority[int]()},
		},
		{
			name: "with multiple options",
			cmp:  intCmp,
			opts: []pq.Option[int]{pq.WithSafe[int](), pq.WithMaxPriority[int]()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := pq.New(tt.cmp, tt.opts...)
			if tt.wantError {
				require.Error(t, err)
				assert.Nil(t, q)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, q)
			}
		})
	}
}

func TestPriorityQueue_Basic(t *testing.T) {
	q, err := pq.New(intCmp)
	require.NoError(t, err)

	assert.True(t, q.IsEmpty())
	assert.Equal(t, 0, q.Len())

	q.Enqueue(3)
	q.Enqueue(1)
	q.Enqueue(2)
	assert.False(t, q.IsEmpty())
	assert.Equal(t, 3, q.Len())

	val, ok := q.Peek()
	assert.True(t, ok)
	assert.Equal(t, 1, val) // Min priority queue, so smallest value first

	val, ok = q.Dequeue()
	assert.True(t, ok)
	assert.Equal(t, 1, val)
	assert.Equal(t, 2, q.Len())

	assert.ElementsMatch(t, []int{2, 3}, q.Values())

	q.Clear()
	assert.True(t, q.IsEmpty())
	assert.Equal(t, 0, q.Len())
}

func TestPriorityQueue_MaxPriority(t *testing.T) {
	q, err := pq.New(intCmp, pq.WithMaxPriority[int]())
	require.NoError(t, err)

	nums := []int{3, 1, 4, 1, 5, 9, 2, 6}
	for _, num := range nums {
		q.Enqueue(num)
	}

	// Verify max priority behavior
	val, ok := q.Peek()
	assert.True(t, ok)
	assert.Equal(t, 9, val) // Max value should be first

	assert.Equal(t, []int{9, 6, 5, 4, 3, 2, 1, 1}, q.Values())
}

func TestPriorityQueue_EmptyOperations(t *testing.T) {
	q, err := pq.New(intCmp)
	require.NoError(t, err)

	val, ok := q.Peek()
	assert.False(t, ok)
	assert.Zero(t, val)

	val, ok = q.Dequeue()
	assert.False(t, ok)
	assert.Zero(t, val)

	assert.Empty(t, q.Values())
}

func TestPriorityQueue_Clone(t *testing.T) {
	q, err := pq.New(intCmp)
	require.NoError(t, err)

	nums := []int{3, 1, 4, 1, 5}
	for _, num := range nums {
		q.Enqueue(num)
	}

	clone := q.Clone()

	assert.Equal(t, q.Len(), clone.Len())
	assert.ElementsMatch(t, q.Values(), clone.Values())

	q.Enqueue(9)
	assert.NotEqual(t, q.Len(), clone.Len())
}

func TestPriorityQueue_String(t *testing.T) {
	q, err := pq.New(intCmp)
	require.NoError(t, err)

	// Empty queue
	assert.Equal(t, "queue:{}", q.String())

	// Add elements
	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)
	assert.Contains(t, q.String(), "1")
	assert.Contains(t, q.String(), "2")
	assert.Contains(t, q.String(), "3")
}

func TestPriorityQueue_Encoding(t *testing.T) {
	q, err := pq.New(intCmp)
	require.NoError(t, err)

	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)

	bytesData, err := json.Marshal(q)
	require.NoError(t, err)

	q2, err := pq.New(intCmp)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(bytesData, q2))

	assert.Equal(t, q.Len(), q2.Len())
	assert.ElementsMatch(t, q.Values(), q2.Values())
}

func TestPriorityQueue_EdgeCases(t *testing.T) {
	q, err := pq.New(intCmp)
	require.NoError(t, err)

	q.Enqueue(1)
	q.Enqueue(1)
	q.Enqueue(1)
	assert.Equal(t, 3, q.Len())

	for !q.IsEmpty() {
		_, ok := q.Dequeue()
		assert.True(t, ok)
	}
	assert.True(t, q.IsEmpty())

	q.Clear()
	assert.True(t, q.IsEmpty())

	clone := q.Clone()
	assert.True(t, clone.IsEmpty())
}
