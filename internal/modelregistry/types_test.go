package modelregistry_test

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
)

type (
	t1 struct{ *model.Empty }
	t4 struct {
		Name string
		*model.Empty
	}
)

type User struct {
	Name   string `json:"name,omitempty"`
	Email  string `json:"email,omitempty"`
	Status uint   `json:"status,omitempty" gorm:"type:smallint;default:1;comment:status(0: disabled, 1: enabled)"`
	model.Base
}

type QueryableUser struct {
	Name string `json:"name,omitempty"`

	model.Query
	model.Base
}

type PaginatableUser struct {
	Name string `json:"name,omitempty"`

	model.Pagination
	model.Base
}

type CursorableUser struct {
	Name string `json:"name,omitempty"`

	model.Cursor
	model.Base
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
	require.NotImplements(t, (*model.Queryable)(nil), new(User))
	require.Implements(t, (*model.Queryable)(nil), new(QueryableUser))
	require.Implements(t, (*model.Queryable)(nil), QueryableUser{})

	require.Implements(t, (*model.Paginatable)(nil), new(QueryableUser))
	require.Implements(t, (*model.Cursorable)(nil), new(QueryableUser))

	require.NotImplements(t, (*model.Queryable)(nil), new(PaginatableUser))
	require.Implements(t, (*model.Paginatable)(nil), new(PaginatableUser))
	require.NotImplements(t, (*model.Cursorable)(nil), new(PaginatableUser))

	require.NotImplements(t, (*model.Queryable)(nil), new(CursorableUser))
	require.NotImplements(t, (*model.Paginatable)(nil), new(CursorableUser))
	require.Implements(t, (*model.Cursorable)(nil), new(CursorableUser))
}

func TestIsEmpty(t *testing.T) {
	type t1 string
	type t2 int
	type t3 struct{}
	type t4 struct{ model.Empty }
	type t5 struct{ *model.Empty }
	type t6 struct{ model.Any }
	type t7 struct{ *model.Any }
	type t8 struct {
		model.Empty
		model.Any
	}
	type t9 struct {
		*model.Empty
		model.Any
	}
	type t10 struct {
		model.Empty
		*model.Any
	}
	type t11 struct {
		model.Empty
		*model.Any
	}
	type t12 struct{ _ string }
	type t13 struct {
		_ string
		model.Empty
	}
	type t14 struct {
		_ string
		model.Any
	}
	type t15 = model.Empty
	type t16 = model.Any

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
	type t4 struct{ model.Empty }
	type t5 struct{ model.Any }
	type t6 struct{ model.Base }

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
