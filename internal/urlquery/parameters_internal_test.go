package urlquery

import (
	"reflect"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

// parameterTestModel declares a field of each kind that decides which
// parameters a list takes: a scalar column, a time column no bare key decodes
// into, a field that is no column, a column hidden from JSON though tagged
// and a column without a query tag, beside the framework's parameters and
// the columns of model.Base.
type parameterTestModel struct {
	Title     string    `json:"title" query:"title"`
	ExpiresAt time.Time `json:"expires_at" query:"expires_at"`
	Scope     string    `json:"scope" gorm:"-" query:"scope"`
	Secret    string    `json:"-" query:"secret"`
	Note      string    `json:"note"`

	modelregistry.Query
	modelregistry.Base
}

// plainParameterTestModel pages by offset without model.Query, so no column
// takes an operator.
type plainParameterTestModel struct {
	Title string `json:"title" query:"title"`

	modelregistry.Pagination
	modelregistry.Base
}

// TestParameters pins the parameters a list takes, in the order of the
// fields declaring them: a bare key for every field Decode fills, the
// framework's own parameters among them, the operators Filters takes on the
// column of each, and the columns Filters alone reads, a column no bare key
// decodes into under its operators alone and a framework timestamp under its
// bare key too. A column clients may not name takes none, and a model
// without model.Query takes no operator and no framework timestamp.
func TestParameters(t *testing.T) {
	all := types.FilterOps()
	times := []types.FilterOp{
		types.FilterOpEq, types.FilterOpNe,
		types.FilterOpGt, types.FilterOpGte, types.FilterOpLt, types.FilterOpLte,
		types.FilterOpIsNull,
	}

	t.Run("Queryable", func(t *testing.T) {
		params, err := Parameters(reflect.TypeFor[*parameterTestModel]())
		require.NoError(t, err)
		require.Equal(t, []Parameter{
			{Name: "title", Index: []int{0}, Bare: true, Ops: all},
			{Name: "expires_at", Index: []int{1}, Ops: times},
			{Name: "scope", Index: []int{2}, Bare: true},
			{Name: "_page", Index: []int{5, 0, 0}, Bare: true},
			{Name: "_size", Index: []int{5, 0, 1}, Bare: true},
			{Name: "_cursor_value", Index: []int{5, 1, 0}, Bare: true},
			{Name: "_cursor_field", Index: []int{5, 1, 1}, Bare: true},
			{Name: "_cursor_next", Index: []int{5, 1, 2}, Bare: true},
			{Name: "_expand", Index: []int{5, 2}, Bare: true},
			{Name: "_depth", Index: []int{5, 3}, Bare: true},
			{Name: "_sort_by", Index: []int{5, 4}, Bare: true},
			{Name: "id", Index: []int{6, 0}, Bare: true, Ops: all},
			{Name: "created_by", Index: []int{6, 1}, Bare: true, Ops: all},
			{Name: "updated_by", Index: []int{6, 2}, Bare: true, Ops: all},
			{Name: "created_at", Index: []int{6, 3}, Bare: true, Ops: times},
			{Name: "updated_at", Index: []int{6, 4}, Bare: true, Ops: times},
		}, params)
	})

	t.Run("NotQueryable", func(t *testing.T) {
		params, err := Parameters(reflect.TypeFor[*plainParameterTestModel]())
		require.NoError(t, err)
		require.Equal(t, []Parameter{
			{Name: "title", Index: []int{0}, Bare: true},
			{Name: "_page", Index: []int{1, 0}, Bare: true},
			{Name: "_size", Index: []int{1, 1}, Bare: true},
			{Name: "id", Index: []int{2, 0}, Bare: true},
			{Name: "created_by", Index: []int{2, 1}, Bare: true},
			{Name: "updated_by", Index: []int{2, 2}, Bare: true},
		}, params)
	})

	t.Run("ColumnTypesTakeTheirOperators", func(t *testing.T) {
		params, err := Parameters(reflect.TypeFor[*filterTestModel]())
		require.NoError(t, err)
		byName := make(map[string]Parameter, len(params))
		for _, param := range params {
			byName[param.Name] = param
		}
		require.Equal(t, []types.FilterOp{types.FilterOpEq, types.FilterOpNe, types.FilterOpIsNull}, byName["enabled"].Ops)
		require.True(t, byName["enabled"].Bare)
		require.Equal(t, []types.FilterOp{
			types.FilterOpLike, types.FilterOpNotLike, types.FilterOpStartsWith, types.FilterOpEndsWith,
			types.FilterOpIsNull,
		}, byName["meta"].Ops)
		require.False(t, byName["meta"].Bare, "a JSON document decodes into no field")
		require.Equal(t, all, byName["age"].Ops)
		require.Equal(t, times, byName["day"].Ops, "a date is filtered by the comparisons alone")
		require.NotContains(t, byName, "note")
		require.NotContains(t, byName, "secret")
		require.NotContains(t, byName, "deleted_at")
	})

	t.Run("EmbeddedColumnsGoByTheirPath", func(t *testing.T) {
		params, err := Parameters(reflect.TypeFor[*namedEmbeddingTestModel]())
		require.NoError(t, err)
		names := make([]string, 0, len(params))
		for _, param := range params {
			names = append(names, param.Name)
		}
		require.Subset(t, names, []string{"label", "first.label", "second.label", "third.label"})
		require.NotContains(t, names, "first", "a struct embedded by name is no parameter of its own")

		params, err = Parameters(reflect.TypeFor[*pointerEmbeddingTestModel]())
		require.NoError(t, err)
		require.Contains(t, params, Parameter{Name: "code", Index: []int{0, 0}, Bare: true, Ops: all},
			"a struct embedded through a pointer promotes its column as one embedded by value does")
	})
}
