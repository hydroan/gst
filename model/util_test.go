package model_test

import (
	"testing"

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

func TestAreTypesEqual(t *testing.T) {
	require.True(t, model.AreTypesEqual[*User, *User, *User]())
	require.False(t, model.AreTypesEqual[*User, User, *User]())
	require.False(t, model.AreTypesEqual[*User, *User, User]())
	require.False(t, model.AreTypesEqual[*User, User, User]())
	require.False(t, model.AreTypesEqual[*User, string, *User]())
	require.False(t, model.AreTypesEqual[*User, *User, int]())
	require.False(t, model.AreTypesEqual[t1, t1, t1]())
	require.True(t, model.AreTypesEqual[t4, t4, t4]())
	require.False(t, model.AreTypesEqual[t1, *User, User]())
	require.False(t, model.AreTypesEqual[t1, int, *string]())
}

func BenchmarkAreTypesEqual(b *testing.B) {
	b.Run("test1", func(b *testing.B) {
		for b.Loop() {
			model.AreTypesEqual[*User, *User, *User]()
		}
	})
	b.Run("test2", func(b *testing.B) {
		for b.Loop() {
			model.AreTypesEqual[*User, User, *User]()
		}
	})
	b.Run("test3", func(b *testing.B) {
		for b.Loop() {
			model.AreTypesEqual[*User, *User, User]()
		}
	})
	b.Run("test4", func(b *testing.B) {
		for b.Loop() {
			model.AreTypesEqual[*User, User, User]()
		}
	})
	b.Run("test6", func(b *testing.B) {
		for b.Loop() {
			model.AreTypesEqual[*User, string, *User]()
		}
	})
	b.Run("test7", func(b *testing.B) {
		for b.Loop() {
			model.AreTypesEqual[*User, *User, int]()
		}
	})
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

	require.True(t, model.IsEmpty[t1]())
	require.True(t, model.IsEmpty[t2]())
	require.True(t, model.IsEmpty[t3]())
	require.True(t, model.IsEmpty[t4]())
	require.True(t, model.IsEmpty[t5]())
	require.True(t, model.IsEmpty[t6]())
	require.True(t, model.IsEmpty[t7]())
	require.True(t, model.IsEmpty[t8]())
	require.True(t, model.IsEmpty[t9]())
	require.True(t, model.IsEmpty[t10]())
	require.True(t, model.IsEmpty[t11]())
	require.False(t, model.IsEmpty[t12]())
	require.False(t, model.IsEmpty[t13]())
	require.False(t, model.IsEmpty[t14]())
	require.True(t, model.IsEmpty[t15]())
	require.True(t, model.IsEmpty[*t15]())
	require.True(t, model.IsEmpty[t16]())
	require.True(t, model.IsEmpty[*t16]())
}

func TestIsValid(t *testing.T) {
	type t1 string
	type t2 int
	type t3 struct{}
	type t4 struct{ model.Empty }
	type t5 struct{ model.Any }
	type t6 struct{ model.Base }

	require.False(t, model.IsValid[t1]())
	require.False(t, model.IsValid[*t1]())
	require.False(t, model.IsValid[t2]())
	require.False(t, model.IsValid[*t2]())
	require.False(t, model.IsValid[t3]())
	require.False(t, model.IsValid[*t3]())
	require.False(t, model.IsValid[t4]())
	require.False(t, model.IsValid[*t4]())
	require.False(t, model.IsValid[t5]())
	require.False(t, model.IsValid[*t5]())
	require.False(t, model.IsValid[t6]())
	require.True(t, model.IsValid[*t6]())
}

func BenchmarkIsModelEmpty(b *testing.B) {
	b.Run("test", func(b *testing.B) {
		for b.Loop() {
			_ = model.IsEmpty[t1]()
		}
	})
}
