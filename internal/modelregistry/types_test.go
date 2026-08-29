package modelregistry_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

type (
	t1 struct{ *modelregistry.Empty }
	t4 struct {
		Name string
		*modelregistry.Empty
	}
)

type User struct {
	Name   string `json:"name,omitempty"`
	Email  string `json:"email,omitempty"`
	Status uint   `json:"status,omitempty" gorm:"type:smallint;default:1;comment:status(0: disabled, 1: enabled)"`
	modelregistry.Base
}

type QueryableUser struct {
	Name string `json:"name,omitempty"`

	modelregistry.Query
	modelregistry.Base
}

type PaginatableUser struct {
	Name string `json:"name,omitempty"`

	modelregistry.Pagination
	modelregistry.Base
}

type CursorableUser struct {
	Name string `json:"name,omitempty"`

	modelregistry.Cursor
	modelregistry.Base
}

// markerMethodSpoofUser declares an exported method matching the historical
// marker name. The sealed marker interfaces must not treat it as opting in.
type markerMethodSpoofUser struct {
	Name string `json:"name,omitempty"`

	modelregistry.Base
}

func (markerMethodSpoofUser) QueryEnabled() {}

// virtualSample mirrors a virtual resource: real query fields plus the Empty
// marker, but no Base and no table.
type virtualSample struct {
	Name string `json:"name,omitempty"`

	modelregistry.Query
	modelregistry.Empty
}

func TestAreTypesEqual(t *testing.T) {
	require.True(t, modelregistry.AreTypesEqual[*User, *User, *User]())
	require.False(t, modelregistry.AreTypesEqual[*User, User, *User]())
	require.False(t, modelregistry.AreTypesEqual[*User, *User, User]())
	require.False(t, modelregistry.AreTypesEqual[*User, User, User]())
	require.False(t, modelregistry.AreTypesEqual[*User, string, *User]())
	require.False(t, modelregistry.AreTypesEqual[*User, *User, int]())
	require.False(t, modelregistry.AreTypesEqual[t1, t1, t1]())
	require.True(t, modelregistry.AreTypesEqual[t4, t4, t4]())
	require.False(t, modelregistry.AreTypesEqual[t1, *User, User]())
	require.False(t, modelregistry.AreTypesEqual[t1, int, *string]())
}

func BenchmarkAreTypesEqual(b *testing.B) {
	b.Run("test1", func(b *testing.B) {
		for b.Loop() {
			modelregistry.AreTypesEqual[*User, *User, *User]()
		}
	})
	b.Run("test2", func(b *testing.B) {
		for b.Loop() {
			modelregistry.AreTypesEqual[*User, User, *User]()
		}
	})
	b.Run("test3", func(b *testing.B) {
		for b.Loop() {
			modelregistry.AreTypesEqual[*User, *User, User]()
		}
	})
	b.Run("test4", func(b *testing.B) {
		for b.Loop() {
			modelregistry.AreTypesEqual[*User, User, User]()
		}
	})
	b.Run("test6", func(b *testing.B) {
		for b.Loop() {
			modelregistry.AreTypesEqual[*User, string, *User]()
		}
	})
	b.Run("test7", func(b *testing.B) {
		for b.Loop() {
			modelregistry.AreTypesEqual[*User, *User, int]()
		}
	})
}

func TestQueryable(t *testing.T) {
	require.False(t, modelregistry.IsQueryable(new(User)))
	require.True(t, modelregistry.IsQueryable(new(QueryableUser)))
	require.True(t, modelregistry.IsQueryable(QueryableUser{}))

	require.True(t, modelregistry.IsPaginatable(new(QueryableUser)))
	require.True(t, modelregistry.IsCursorable(new(QueryableUser)))

	require.False(t, modelregistry.IsQueryable(new(PaginatableUser)))
	require.True(t, modelregistry.IsPaginatable(new(PaginatableUser)))
	require.False(t, modelregistry.IsCursorable(new(PaginatableUser)))

	require.False(t, modelregistry.IsQueryable(new(CursorableUser)))
	require.False(t, modelregistry.IsPaginatable(new(CursorableUser)))
	require.True(t, modelregistry.IsCursorable(new(CursorableUser)))

	// Embedding the framework query structs is the only opt-in path.
	require.False(t, modelregistry.IsQueryable(new(markerMethodSpoofUser)))
	require.False(t, modelregistry.IsPaginatable(new(markerMethodSpoofUser)))
	require.False(t, modelregistry.IsCursorable(new(markerMethodSpoofUser)))
}

func TestIsVirtual(t *testing.T) {
	// Embedding Empty is the opt-in, whether by value, by pointer, or beside
	// other fields; IsEmpty stays a separate, narrower predicate.
	require.True(t, modelregistry.IsVirtual(new(virtualSample)))
	require.True(t, modelregistry.IsVirtual(new(t1)))
	require.True(t, modelregistry.IsVirtual(new(t4)))
	require.True(t, modelregistry.IsVirtual(new(modelregistry.Empty)))

	// The marker follows Empty's pointer receivers, so only the pointer shape
	// models actually flow through the framework in carries it.
	require.False(t, modelregistry.IsVirtual(virtualSample{}))

	// Table-backed models never opt in.
	require.False(t, modelregistry.IsVirtual(new(User)))
	require.False(t, modelregistry.IsVirtual(new(QueryableUser)))
}

func TestIsQueryMarkerType(t *testing.T) {
	require.True(t, modelregistry.IsQueryMarkerType(reflect.TypeFor[modelregistry.Query]()))
	require.True(t, modelregistry.IsQueryMarkerType(reflect.TypeFor[*modelregistry.Query]()))
	require.True(t, modelregistry.IsQueryMarkerType(reflect.TypeFor[modelregistry.Pagination]()))
	require.True(t, modelregistry.IsQueryMarkerType(reflect.TypeFor[modelregistry.Cursor]()))

	// Nested structs that embed a marker struct carry framework query
	// parameters as well, so they are also excluded from SQL conditions.
	require.True(t, modelregistry.IsQueryMarkerType(reflect.TypeFor[QueryableUser]()))

	require.False(t, modelregistry.IsQueryMarkerType(nil))
	require.False(t, modelregistry.IsQueryMarkerType(reflect.TypeFor[User]()))
	require.False(t, modelregistry.IsQueryMarkerType(reflect.TypeFor[markerMethodSpoofUser]()))
	require.False(t, modelregistry.IsQueryMarkerType(reflect.TypeFor[string]()))
}

func TestIsEmpty(t *testing.T) {
	type t1 string
	type t2 int
	type t3 struct{}
	type t4 struct{ modelregistry.Empty }
	type t5 struct{ *modelregistry.Empty }
	type t6 struct{ _ string }
	type t7 struct {
		_ string
		modelregistry.Empty
	}
	type t8 = modelregistry.Empty

	require.True(t, modelregistry.IsEmpty[t1]())
	require.True(t, modelregistry.IsEmpty[t2]())
	require.True(t, modelregistry.IsEmpty[t3]())
	require.True(t, modelregistry.IsEmpty[t4]())
	require.True(t, modelregistry.IsEmpty[t5]())
	require.False(t, modelregistry.IsEmpty[t6]())
	require.False(t, modelregistry.IsEmpty[t7]())
	require.True(t, modelregistry.IsEmpty[t8]())
	require.True(t, modelregistry.IsEmpty[*t8]())
}

func TestIsValid(t *testing.T) {
	type t1 string
	type t2 int
	type t3 struct{}
	type t4 struct{ modelregistry.Empty }
	type t5 struct{ modelregistry.Base }

	require.False(t, modelregistry.IsValid[t1]())
	require.False(t, modelregistry.IsValid[*t1]())
	require.False(t, modelregistry.IsValid[t2]())
	require.False(t, modelregistry.IsValid[*t2]())
	require.False(t, modelregistry.IsValid[t3]())
	require.False(t, modelregistry.IsValid[*t3]())
	require.False(t, modelregistry.IsValid[t4]())
	require.False(t, modelregistry.IsValid[*t4]())
	require.False(t, modelregistry.IsValid[t5]())
	require.True(t, modelregistry.IsValid[*t5]())
}

func BenchmarkIsModelEmpty(b *testing.B) {
	b.Run("test", func(b *testing.B) {
		for b.Loop() {
			_ = modelregistry.IsEmpty[t1]()
		}
	})
}

func TestTableDoneSignalsWaiters(t *testing.T) {
	// The queue and its wakeup are package-level state, so every subtest starts
	// from a drained signal and hands back a drained one.
	drain := func() {
		for {
			select {
			case <-modelregistry.TablesChanged():
			default:
				return
			}
		}
	}
	drain()
	t.Cleanup(drain)

	// enqueue counts one model as pending the way the database runtime sees it:
	// queued by EnqueueTable, then taken off the queue and prepared.
	enqueue := func(t *testing.T, count int) {
		t.Helper()
		for range count {
			modelregistry.EnqueueTable(&IndexedSample{})
			<-modelregistry.TableChan
		}
	}

	t.Run("a_finished_table_wakes_a_waiter", func(t *testing.T) {
		drain()
		enqueue(t, 1)
		require.Equal(t, 1, modelregistry.TablesPending())

		modelregistry.TableDone()

		require.Equal(t, 0, modelregistry.TablesPending())
		select {
		case <-modelregistry.TablesChanged():
		default:
			t.Fatal("TableDone left no wakeup for a waiter")
		}
	})

	t.Run("wakeups_coalesce_and_never_block", func(t *testing.T) {
		drain()
		// More finished tables than the signal buffer holds: the sends have to
		// fall through rather than block, because TableDone runs on the
		// goroutine that prepares the tables.
		enqueue(t, 4)
		for range 4 {
			modelregistry.TableDone()
		}

		require.Equal(t, 0, modelregistry.TablesPending())
		// One wakeup stands in for all four: it only says the count is worth
		// reading again, and the count is what a waiter trusts.
		<-modelregistry.TablesChanged()
		select {
		case <-modelregistry.TablesChanged():
			t.Fatal("the wakeups should have coalesced into one")
		default:
		}
	})
}
