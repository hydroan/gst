package types_test

import (
	"testing"

	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

// sampleStatus is a named string type, standing in for a model enum.
type sampleStatus string

const (
	sampleStatusActive  sampleStatus = "active"
	sampleStatusRemoved sampleStatus = "removed"
)

func TestColumnBuildsFilters(t *testing.T) {
	age := types.Column[int]{Name: "age"}
	status := types.Column[sampleStatus]{Name: "status"}
	name := types.Column[string]{Name: "name"}

	tests := []struct {
		label string
		got   types.Filter
		want  types.Filter
	}{
		{"Eq", age.Eq(18), types.Filter{Column: "age", Op: types.FilterOpEq, Value: 18}},
		{"Ne", age.Ne(18), types.Filter{Column: "age", Op: types.FilterOpNe, Value: 18}},
		{"Gt", age.Gt(18), types.Filter{Column: "age", Op: types.FilterOpGt, Value: 18}},
		{"Gte", age.Gte(18), types.Filter{Column: "age", Op: types.FilterOpGte, Value: 18}},
		{"Lt", age.Lt(18), types.Filter{Column: "age", Op: types.FilterOpLt, Value: 18}},
		{"Lte", age.Lte(18), types.Filter{Column: "age", Op: types.FilterOpLte, Value: 18}},
		{
			"In",
			status.In(sampleStatusActive, sampleStatusRemoved),
			types.Filter{Column: "status", Op: types.FilterOpIn, Value: []sampleStatus{sampleStatusActive, sampleStatusRemoved}},
		},
		{
			"NotIn",
			status.NotIn(sampleStatusRemoved),
			types.Filter{Column: "status", Op: types.FilterOpNotIn, Value: []sampleStatus{sampleStatusRemoved}},
		},
		{"Like", name.Like("sam"), types.Filter{Column: "name", Op: types.FilterOpLike, Value: "sam"}},
		{"NotLike", name.NotLike("sam"), types.Filter{Column: "name", Op: types.FilterOpNotLike, Value: "sam"}},
		{"StartsWith", name.StartsWith("sa"), types.Filter{Column: "name", Op: types.FilterOpStartsWith, Value: "sa"}},
		{"EndsWith", name.EndsWith("le"), types.Filter{Column: "name", Op: types.FilterOpEndsWith, Value: "le"}},
		{"IsNull", name.IsNull(), types.Filter{Column: "name", Op: types.FilterOpIsNull, Value: true}},
		{"NotNull", name.NotNull(), types.Filter{Column: "name", Op: types.FilterOpIsNull, Value: false}},
		{"Regex", name.Regex("^sa"), types.Filter{Column: "name", Op: types.FilterOpRegex, Value: "^sa"}},
		{"NotRegex", name.NotRegex("^sa"), types.Filter{Column: "name", Op: types.FilterOpNotRegex, Value: "^sa"}},
		{"JSONContains", name.JSONContains("sam"), types.Filter{Column: "name", Op: types.FilterOpJSONContains, Value: "sam"}},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}

func TestColumnInWithoutValues(t *testing.T) {
	status := types.Column[sampleStatus]{Name: "status"}
	// A variadic call with no arguments yields a nil slice. It still carries
	// the slice type, so the database layer treats it as an empty set and
	// matches nothing rather than widening the query.
	require.Equal(t,
		types.Filter{Column: "status", Op: types.FilterOpIn, Value: []sampleStatus(nil)},
		status.In())
}
