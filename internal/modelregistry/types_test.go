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

type UnsafeQueryableUser struct {
	Name string `json:"name,omitempty"`

	modelregistry.Query
	modelregistry.UnsafeQuery
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
	require.False(t, modelregistry.IsUnsafeQueryable(new(QueryableUser)))

	require.True(t, modelregistry.IsQueryable(new(UnsafeQueryableUser)))
	require.True(t, modelregistry.IsUnsafeQueryable(new(UnsafeQueryableUser)))
	require.True(t, modelregistry.IsUnsafeQueryable(UnsafeQueryableUser{}))

	require.False(t, modelregistry.IsQueryable(new(PaginatableUser)))
	require.True(t, modelregistry.IsPaginatable(new(PaginatableUser)))
	require.False(t, modelregistry.IsCursorable(new(PaginatableUser)))

	require.False(t, modelregistry.IsQueryable(new(CursorableUser)))
	require.False(t, modelregistry.IsPaginatable(new(CursorableUser)))
	require.True(t, modelregistry.IsCursorable(new(CursorableUser)))

	// Embedding the framework query structs is the only opt-in path.
	require.False(t, modelregistry.IsQueryable(new(markerMethodSpoofUser)))
	require.False(t, modelregistry.IsUnsafeQueryable(new(markerMethodSpoofUser)))
	require.False(t, modelregistry.IsPaginatable(new(markerMethodSpoofUser)))
	require.False(t, modelregistry.IsCursorable(new(markerMethodSpoofUser)))
}

func TestIsQueryMarkerType(t *testing.T) {
	require.True(t, modelregistry.IsQueryMarkerType(reflect.TypeFor[modelregistry.Query]()))
	require.True(t, modelregistry.IsQueryMarkerType(reflect.TypeFor[*modelregistry.Query]()))
	require.True(t, modelregistry.IsQueryMarkerType(reflect.TypeFor[modelregistry.UnsafeQuery]()))
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
	type t6 struct{ modelregistry.Any }
	type t7 struct{ *modelregistry.Any }
	type t8 struct {
		modelregistry.Empty
		modelregistry.Any
	}
	type t9 struct {
		*modelregistry.Empty
		modelregistry.Any
	}
	type t10 struct {
		modelregistry.Empty
		*modelregistry.Any
	}
	type t11 struct {
		modelregistry.Empty
		*modelregistry.Any
	}
	type t12 struct{ _ string }
	type t13 struct {
		_ string
		modelregistry.Empty
	}
	type t14 struct {
		_ string
		modelregistry.Any
	}
	type t15 = modelregistry.Empty
	type t16 = modelregistry.Any

	require.True(t, modelregistry.IsEmpty[t1]())
	require.True(t, modelregistry.IsEmpty[t2]())
	require.True(t, modelregistry.IsEmpty[t3]())
	require.True(t, modelregistry.IsEmpty[t4]())
	require.True(t, modelregistry.IsEmpty[t5]())
	require.True(t, modelregistry.IsEmpty[t6]())
	require.True(t, modelregistry.IsEmpty[t7]())
	require.True(t, modelregistry.IsEmpty[t8]())
	require.True(t, modelregistry.IsEmpty[t9]())
	require.True(t, modelregistry.IsEmpty[t10]())
	require.True(t, modelregistry.IsEmpty[t11]())
	require.False(t, modelregistry.IsEmpty[t12]())
	require.False(t, modelregistry.IsEmpty[t13]())
	require.False(t, modelregistry.IsEmpty[t14]())
	require.True(t, modelregistry.IsEmpty[t15]())
	require.True(t, modelregistry.IsEmpty[*t15]())
	require.True(t, modelregistry.IsEmpty[t16]())
	require.True(t, modelregistry.IsEmpty[*t16]())
}

func TestIsValid(t *testing.T) {
	type t1 string
	type t2 int
	type t3 struct{}
	type t4 struct{ modelregistry.Empty }
	type t5 struct{ modelregistry.Any }
	type t6 struct{ modelregistry.Base }

	require.False(t, modelregistry.IsValid[t1]())
	require.False(t, modelregistry.IsValid[*t1]())
	require.False(t, modelregistry.IsValid[t2]())
	require.False(t, modelregistry.IsValid[*t2]())
	require.False(t, modelregistry.IsValid[t3]())
	require.False(t, modelregistry.IsValid[*t3]())
	require.False(t, modelregistry.IsValid[t4]())
	require.False(t, modelregistry.IsValid[*t4]())
	require.False(t, modelregistry.IsValid[t5]())
	require.False(t, modelregistry.IsValid[*t5]())
	require.False(t, modelregistry.IsValid[t6]())
	require.True(t, modelregistry.IsValid[*t6]())
}

func BenchmarkIsModelEmpty(b *testing.B) {
	b.Run("test", func(b *testing.B) {
		for b.Loop() {
			_ = modelregistry.IsEmpty[t1]()
		}
	})
}
