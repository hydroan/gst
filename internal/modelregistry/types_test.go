package modelregistry_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

type (
	pointerMarkerOnly       struct{ *modelregistry.Empty }
	pointerMarkerBesideName struct {
		Name string
		*modelregistry.Empty
	}
)

type Record struct {
	Name   string `json:"name,omitempty"`
	Code   string `json:"code,omitempty"`
	Status uint   `json:"status,omitempty" gorm:"type:smallint;default:1;comment:status(0: disabled, 1: enabled)"`
	modelregistry.Base
}

type QueryableRecord struct {
	Name string `json:"name,omitempty"`

	modelregistry.Query
	modelregistry.Base
}

type PaginatableRecord struct {
	Name string `json:"name,omitempty"`

	modelregistry.Pagination
	modelregistry.Base
}

type CursorableRecord struct {
	Name string `json:"name,omitempty"`

	modelregistry.Cursor
	modelregistry.Base
}

// markerMethodSpoofRecord declares an exported QueryEnabled method, a name that
// reads like a marker. The sealed marker interfaces must not treat it as
// opting in.
type markerMethodSpoofRecord struct {
	Name string `json:"name,omitempty"`

	modelregistry.Base
}

func (markerMethodSpoofRecord) QueryEnabled() {}

// virtualSample mirrors a virtual resource: real query fields plus the Empty
// marker, but no Base and no table.
type virtualSample struct {
	Name string `json:"name,omitempty"`

	modelregistry.Query
	modelregistry.Empty
}

func TestAreTypesEqual(t *testing.T) {
	require.True(t, modelregistry.AreTypesEqual[*Record, *Record, *Record]())
	require.False(t, modelregistry.AreTypesEqual[*Record, Record, *Record]())
	require.False(t, modelregistry.AreTypesEqual[*Record, *Record, Record]())
	require.False(t, modelregistry.AreTypesEqual[*Record, Record, Record]())
	require.False(t, modelregistry.AreTypesEqual[*Record, string, *Record]())
	require.False(t, modelregistry.AreTypesEqual[*Record, *Record, int]())
	require.False(t, modelregistry.AreTypesEqual[pointerMarkerOnly, pointerMarkerOnly, pointerMarkerOnly]())
	require.True(t, modelregistry.AreTypesEqual[pointerMarkerBesideName, pointerMarkerBesideName, pointerMarkerBesideName]())
	require.False(t, modelregistry.AreTypesEqual[pointerMarkerOnly, *Record, Record]())
	require.False(t, modelregistry.AreTypesEqual[pointerMarkerOnly, int, *string]())
}

func BenchmarkAreTypesEqual(b *testing.B) {
	b.Run("SameModel", func(b *testing.B) {
		for b.Loop() {
			modelregistry.AreTypesEqual[*Record, *Record, *Record]()
		}
	})
	b.Run("ValueRequest", func(b *testing.B) {
		for b.Loop() {
			modelregistry.AreTypesEqual[*Record, Record, *Record]()
		}
	})
	b.Run("ValueResponse", func(b *testing.B) {
		for b.Loop() {
			modelregistry.AreTypesEqual[*Record, *Record, Record]()
		}
	})
	b.Run("ValueRequestAndResponse", func(b *testing.B) {
		for b.Loop() {
			modelregistry.AreTypesEqual[*Record, Record, Record]()
		}
	})
	b.Run("StringRequest", func(b *testing.B) {
		for b.Loop() {
			modelregistry.AreTypesEqual[*Record, string, *Record]()
		}
	})
	b.Run("IntResponse", func(b *testing.B) {
		for b.Loop() {
			modelregistry.AreTypesEqual[*Record, *Record, int]()
		}
	})
}

func TestQueryable(t *testing.T) {
	require.False(t, modelregistry.IsQueryable(new(Record)))
	require.True(t, modelregistry.IsQueryable(new(QueryableRecord)))
	require.True(t, modelregistry.IsQueryable(QueryableRecord{}))

	require.True(t, modelregistry.IsPaginatable(new(QueryableRecord)))
	require.True(t, modelregistry.IsCursorable(new(QueryableRecord)))

	require.False(t, modelregistry.IsQueryable(new(PaginatableRecord)))
	require.True(t, modelregistry.IsPaginatable(new(PaginatableRecord)))
	require.False(t, modelregistry.IsCursorable(new(PaginatableRecord)))

	require.False(t, modelregistry.IsQueryable(new(CursorableRecord)))
	require.False(t, modelregistry.IsPaginatable(new(CursorableRecord)))
	require.True(t, modelregistry.IsCursorable(new(CursorableRecord)))

	// Embedding the framework query structs is the only opt-in path.
	require.False(t, modelregistry.IsQueryable(new(markerMethodSpoofRecord)))
	require.False(t, modelregistry.IsPaginatable(new(markerMethodSpoofRecord)))
	require.False(t, modelregistry.IsCursorable(new(markerMethodSpoofRecord)))
}

func TestIsVirtual(t *testing.T) {
	// Embedding Empty is the opt-in, alone or beside other fields; IsEmpty
	// stays a separate, narrower predicate. An embedded *Empty promotes the
	// marker too, though Register and gg gen reject that form.
	require.True(t, modelregistry.IsVirtual(new(virtualSample)))
	require.True(t, modelregistry.IsVirtual(new(pointerMarkerOnly)))
	require.True(t, modelregistry.IsVirtual(new(pointerMarkerBesideName)))
	require.True(t, modelregistry.IsVirtual(new(modelregistry.Empty)))

	// The marker follows Empty's pointer receivers, so only the pointer shape
	// models actually flow through the framework in carries it.
	require.False(t, modelregistry.IsVirtual(virtualSample{}))

	// Table-backed models never opt in.
	require.False(t, modelregistry.IsVirtual(new(Record)))
	require.False(t, modelregistry.IsVirtual(new(QueryableRecord)))
}

func TestIsEmpty(t *testing.T) {
	type t1 struct{}
	type t2 struct{ modelregistry.Empty }
	type t3 struct{ *modelregistry.Empty }
	type t4 = modelregistry.Empty
	type t5 struct{ _ string }
	type t6 struct {
		_ string
		modelregistry.Empty
	}
	type t7 string
	type t8 int
	type t9 []t5
	type t10 map[string]t5

	require.True(t, modelregistry.IsEmpty[t1]())
	require.True(t, modelregistry.IsEmpty[t2]())
	require.True(t, modelregistry.IsEmpty[t3]())
	require.True(t, modelregistry.IsEmpty[t4]())
	require.True(t, modelregistry.IsEmpty[*t4]())

	// A struct with a field besides the markers carries it, and a type of any
	// other kind carries its value: a string, a number, the elements of a
	// slice or of a map.
	require.False(t, modelregistry.IsEmpty[t5]())
	require.False(t, modelregistry.IsEmpty[t6]())
	require.False(t, modelregistry.IsEmpty[t7]())
	require.False(t, modelregistry.IsEmpty[t8]())
	require.False(t, modelregistry.IsEmpty[t9]())
	require.False(t, modelregistry.IsEmpty[t10]())
}

func TestIsTableModel(t *testing.T) {
	type t1 struct{ modelregistry.Base }
	type t2 string
	type t3 int
	type t4 struct{}
	type t5 struct{ modelregistry.Empty }
	type t6 struct {
		Name string
		*modelregistry.Empty
	}

	require.True(t, modelregistry.IsTableModel[*t1]())

	require.False(t, modelregistry.IsTableModel[t1]())
	require.False(t, modelregistry.IsTableModel[t2]())
	require.False(t, modelregistry.IsTableModel[*t2]())
	require.False(t, modelregistry.IsTableModel[t3]())
	require.False(t, modelregistry.IsTableModel[*t3]())
	require.False(t, modelregistry.IsTableModel[t4]())
	require.False(t, modelregistry.IsTableModel[*t4]())
	require.False(t, modelregistry.IsTableModel[t5]())
	require.False(t, modelregistry.IsTableModel[*t5]())
	require.False(t, modelregistry.IsTableModel[*t6]())
}

func TestIsFrameworkBase(t *testing.T) {
	require.True(t, modelregistry.IsFrameworkBase(reflect.TypeFor[modelregistry.Base]()))
	require.True(t, modelregistry.IsFrameworkBase(reflect.TypeFor[modelregistry.AutoBase]()))

	require.False(t, modelregistry.IsFrameworkBase(reflect.TypeFor[modelregistry.Empty]()), "the virtual marker carries no fields to write")
	require.False(t, modelregistry.IsFrameworkBase(reflect.TypeFor[*modelregistry.Base]()), "a base is embedded by value")
	require.False(t, modelregistry.IsFrameworkBase(reflect.TypeFor[Record]()))
}

func BenchmarkIsEmpty(b *testing.B) {
	for b.Loop() {
		_ = modelregistry.IsEmpty[pointerMarkerOnly]()
	}
}
