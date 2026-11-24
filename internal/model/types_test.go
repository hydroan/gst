package model_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/model"
	"github.com/stretchr/testify/require"
)

type (
	pointerMarkerOnly       struct{ *model.Empty }
	pointerMarkerBesideName struct {
		Name string
		*model.Empty
	}
)

type Record struct {
	Name   string `json:"name,omitempty"`
	Code   string `json:"code,omitempty"`
	Status uint   `json:"status,omitempty" gorm:"type:smallint;default:1;comment:status(0: disabled, 1: enabled)"`
	model.Base
}

type QueryableRecord struct {
	Name string `json:"name,omitempty"`

	model.Query
	model.Base
}

type CursorableRecord struct {
	Name string `json:"name,omitempty"`

	model.Cursor
	model.Base
}

// markerMethodSpoofRecord declares an exported QueryEnabled method, a name that
// reads like a marker. The sealed marker interfaces must not treat it as
// opting in.
type markerMethodSpoofRecord struct {
	Name string `json:"name,omitempty"`

	model.Base
}

func (markerMethodSpoofRecord) QueryEnabled() {}

// virtualSample mirrors a virtual resource: real query fields plus the Empty
// marker, but no Base and no table.
type virtualSample struct {
	Name string `json:"name,omitempty"`

	model.Query
	model.Empty
}

func TestAreTypesEqual(t *testing.T) {
	require.True(t, model.AreTypesEqual[*Record, *Record, *Record]())
	require.False(t, model.AreTypesEqual[*Record, Record, *Record]())
	require.False(t, model.AreTypesEqual[*Record, *Record, Record]())
	require.False(t, model.AreTypesEqual[*Record, Record, Record]())
	require.False(t, model.AreTypesEqual[*Record, string, *Record]())
	require.False(t, model.AreTypesEqual[*Record, *Record, int]())
	require.False(t, model.AreTypesEqual[pointerMarkerOnly, pointerMarkerOnly, pointerMarkerOnly]())
	require.True(t, model.AreTypesEqual[pointerMarkerBesideName, pointerMarkerBesideName, pointerMarkerBesideName]())
	require.False(t, model.AreTypesEqual[pointerMarkerOnly, *Record, Record]())
	require.False(t, model.AreTypesEqual[pointerMarkerOnly, int, *string]())
}

func BenchmarkAreTypesEqual(b *testing.B) {
	b.Run("SameModel", func(b *testing.B) {
		for b.Loop() {
			model.AreTypesEqual[*Record, *Record, *Record]()
		}
	})
	b.Run("ValueRequest", func(b *testing.B) {
		for b.Loop() {
			model.AreTypesEqual[*Record, Record, *Record]()
		}
	})
	b.Run("ValueResponse", func(b *testing.B) {
		for b.Loop() {
			model.AreTypesEqual[*Record, *Record, Record]()
		}
	})
	b.Run("ValueRequestAndResponse", func(b *testing.B) {
		for b.Loop() {
			model.AreTypesEqual[*Record, Record, Record]()
		}
	})
	b.Run("StringRequest", func(b *testing.B) {
		for b.Loop() {
			model.AreTypesEqual[*Record, string, *Record]()
		}
	})
	b.Run("IntResponse", func(b *testing.B) {
		for b.Loop() {
			model.AreTypesEqual[*Record, *Record, int]()
		}
	})
}

func TestQueryable(t *testing.T) {
	require.False(t, model.IsQueryable(new(Record)))
	require.True(t, model.IsQueryable(new(QueryableRecord)))
	require.True(t, model.IsQueryable(QueryableRecord{}))

	require.False(t, model.IsCursorable(new(QueryableRecord)), "Query brings no cursor")

	require.False(t, model.IsQueryable(new(CursorableRecord)))
	require.True(t, model.IsCursorable(new(CursorableRecord)))

	// Embedding the framework query structs is the only opt-in path.
	require.False(t, model.IsQueryable(new(markerMethodSpoofRecord)))
	require.False(t, model.IsCursorable(new(markerMethodSpoofRecord)))
}

func TestIsVirtual(t *testing.T) {
	// Embedding Empty is the opt-in, alone or beside other fields; IsEmpty
	// stays a separate, narrower predicate. An embedded *Empty promotes the
	// marker too, though Register and gg gen reject that form.
	require.True(t, model.IsVirtual(new(virtualSample)))
	require.True(t, model.IsVirtual(new(pointerMarkerOnly)))
	require.True(t, model.IsVirtual(new(pointerMarkerBesideName)))
	require.True(t, model.IsVirtual(new(model.Empty)))

	// The marker follows Empty's pointer receivers, so only the pointer shape
	// models actually flow through the framework in carries it.
	require.False(t, model.IsVirtual(virtualSample{}))

	// Table-backed models never opt in.
	require.False(t, model.IsVirtual(new(Record)))
	require.False(t, model.IsVirtual(new(QueryableRecord)))
}

func TestIsEmpty(t *testing.T) {
	type t1 struct{}
	type t2 struct{ model.Empty }
	type t3 struct{ *model.Empty }
	type t4 = model.Empty
	type t5 struct{ _ string }
	type t6 struct {
		_ string
		model.Empty
	}
	type t7 string
	type t8 int
	type t9 []t5
	type t10 map[string]t5

	require.True(t, model.IsEmpty[t1]())
	require.True(t, model.IsEmpty[t2]())
	require.True(t, model.IsEmpty[t3]())
	require.True(t, model.IsEmpty[t4]())
	require.True(t, model.IsEmpty[*t4]())

	// A struct with a field besides the markers carries it, and a type of any
	// other kind carries its value: a string, a number, the elements of a
	// slice or of a map.
	require.False(t, model.IsEmpty[t5]())
	require.False(t, model.IsEmpty[t6]())
	require.False(t, model.IsEmpty[t7]())
	require.False(t, model.IsEmpty[t8]())
	require.False(t, model.IsEmpty[t9]())
	require.False(t, model.IsEmpty[t10]())
}

func TestIsTableModel(t *testing.T) {
	type t1 struct{ model.Base }
	type t2 string
	type t3 int
	type t4 struct{}
	type t5 struct{ model.Empty }
	type t6 struct {
		Name string
		*model.Empty
	}

	require.True(t, model.IsTableModel[*t1]())

	require.False(t, model.IsTableModel[t1]())
	require.False(t, model.IsTableModel[t2]())
	require.False(t, model.IsTableModel[*t2]())
	require.False(t, model.IsTableModel[t3]())
	require.False(t, model.IsTableModel[*t3]())
	require.False(t, model.IsTableModel[t4]())
	require.False(t, model.IsTableModel[*t4]())
	require.False(t, model.IsTableModel[t5]())
	require.False(t, model.IsTableModel[*t5]())
	require.False(t, model.IsTableModel[*t6]())
}

func TestIsFrameworkBase(t *testing.T) {
	require.True(t, model.IsFrameworkBase(reflect.TypeFor[model.Base]()))
	require.True(t, model.IsFrameworkBase(reflect.TypeFor[model.AutoBase]()))

	require.False(t, model.IsFrameworkBase(reflect.TypeFor[model.Empty]()), "the virtual marker carries no fields to write")
	require.False(t, model.IsFrameworkBase(reflect.TypeFor[*model.Base]()), "a base is embedded by value")
	require.False(t, model.IsFrameworkBase(reflect.TypeFor[Record]()))
}

func BenchmarkIsEmpty(b *testing.B) {
	for b.Loop() {
		_ = model.IsEmpty[pointerMarkerOnly]()
	}
}
