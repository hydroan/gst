package types_test

import (
	"testing"

	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

func TestParseFilterOp(t *testing.T) {
	t.Run("AcceptsEveryKnownOperator", func(t *testing.T) {
		for s, want := range map[string]types.FilterOp{
			"eq":         types.FilterOpEq,
			"ne":         types.FilterOpNe,
			"gt":         types.FilterOpGt,
			"gte":        types.FilterOpGte,
			"lt":         types.FilterOpLt,
			"lte":        types.FilterOpLte,
			"in":         types.FilterOpIn,
			"notin":      types.FilterOpNotIn,
			"like":       types.FilterOpLike,
			"notlike":    types.FilterOpNotLike,
			"startswith": types.FilterOpStartsWith,
			"endswith":   types.FilterOpEndsWith,
			"isnull":     types.FilterOpIsNull,
		} {
			op, ok := types.ParseFilterOp(s)
			require.True(t, ok, "operator %q must be recognized", s)
			require.Equal(t, want, op)
		}
	})

	t.Run("RejectsUnknownOperator", func(t *testing.T) {
		for _, s := range []string{"", "eq ", "EQ", "between", "notnull"} {
			_, ok := types.ParseFilterOp(s)
			require.False(t, ok, "operator %q must be rejected", s)
		}
	})

	t.Run("ServiceOnlyOperatorsAreNotParseable", func(t *testing.T) {
		for _, op := range []types.FilterOp{types.FilterOpRegex, types.FilterOpNotRegex, types.FilterOpJSONContains} {
			_, ok := types.ParseFilterOp(string(op))
			require.False(t, ok, "service-only operator %q must not be reachable from URL parsing", op)
		}
	})
}

func TestFilterConstructors(t *testing.T) {
	tests := []struct {
		name string
		got  types.Filter
		want types.Filter
	}{
		{"Eq", types.FilterEq("age", 18), types.Filter{Column: "age", Op: types.FilterOpEq, Value: 18}},
		{"Ne", types.FilterNe("age", 18), types.Filter{Column: "age", Op: types.FilterOpNe, Value: 18}},
		{"Gt", types.FilterGt("age", 18), types.Filter{Column: "age", Op: types.FilterOpGt, Value: 18}},
		{"Gte", types.FilterGte("age", 18), types.Filter{Column: "age", Op: types.FilterOpGte, Value: 18}},
		{"Lt", types.FilterLt("age", 18), types.Filter{Column: "age", Op: types.FilterOpLt, Value: 18}},
		{"Lte", types.FilterLte("age", 18), types.Filter{Column: "age", Op: types.FilterOpLte, Value: 18}},
		{"In", types.FilterIn("id", []string{"a", "b"}), types.Filter{Column: "id", Op: types.FilterOpIn, Value: []string{"a", "b"}}},
		{"NotIn", types.FilterNotIn("id", []int{1, 2}), types.Filter{Column: "id", Op: types.FilterOpNotIn, Value: []int{1, 2}}},
		{"Like", types.FilterLike("name", "sample"), types.Filter{Column: "name", Op: types.FilterOpLike, Value: "sample"}},
		{"NotLike", types.FilterNotLike("name", "sample"), types.Filter{Column: "name", Op: types.FilterOpNotLike, Value: "sample"}},
		{"StartsWith", types.FilterStartsWith("name", "sam"), types.Filter{Column: "name", Op: types.FilterOpStartsWith, Value: "sam"}},
		{"EndsWith", types.FilterEndsWith("name", "ple"), types.Filter{Column: "name", Op: types.FilterOpEndsWith, Value: "ple"}},
		{"IsNull", types.FilterIsNull("expired_at"), types.Filter{Column: "expired_at", Op: types.FilterOpIsNull, Value: true}},
		{"NotNull", types.FilterNotNull("expired_at"), types.Filter{Column: "expired_at", Op: types.FilterOpIsNull, Value: false}},
		{"Regex", types.FilterRegex("name", "^sam"), types.Filter{Column: "name", Op: types.FilterOpRegex, Value: "^sam"}},
		{"NotRegex", types.FilterNotRegex("name", "^sam"), types.Filter{Column: "name", Op: types.FilterOpNotRegex, Value: "^sam"}},
		{"JSONContains", types.FilterJSONContains("tags", "sample"), types.Filter{Column: "tags", Op: types.FilterOpJSONContains, Value: "sample"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}
