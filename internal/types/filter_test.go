package types_test

import (
	"testing"

	"github.com/hydroan/gst/internal/types"
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
		for _, op := range []types.FilterOp{
			types.FilterOpRegex, types.FilterOpNotRegex, types.FilterOpJSONContains,
			types.FilterOpOr, types.FilterOpAnd, types.FilterOpExists,
			types.FilterOpEqCol, types.FilterOpFalse,
		} {
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
		{"Eq", types.FilterEq("age", 18), types.NewFilter("", "age", types.FilterOpEq, 18)},
		{"Ne", types.FilterNe("age", 18), types.NewFilter("", "age", types.FilterOpNe, 18)},
		{"Gt", types.FilterGt("age", 18), types.NewFilter("", "age", types.FilterOpGt, 18)},
		{"Gte", types.FilterGte("age", 18), types.NewFilter("", "age", types.FilterOpGte, 18)},
		{"Lt", types.FilterLt("age", 18), types.NewFilter("", "age", types.FilterOpLt, 18)},
		{"Lte", types.FilterLte("age", 18), types.NewFilter("", "age", types.FilterOpLte, 18)},
		{"In", types.FilterIn("id", []string{"a", "b"}), types.NewFilter("", "id", types.FilterOpIn, []string{"a", "b"})},
		{"NotIn", types.FilterNotIn("id", []int{1, 2}), types.NewFilter("", "id", types.FilterOpNotIn, []int{1, 2})},
		{"Like", types.FilterLike("name", "sample"), types.NewFilter("", "name", types.FilterOpLike, "sample")},
		{"NotLike", types.FilterNotLike("name", "sample"), types.NewFilter("", "name", types.FilterOpNotLike, "sample")},
		{"StartsWith", types.FilterStartsWith("name", "sam"), types.NewFilter("", "name", types.FilterOpStartsWith, "sam")},
		{"EndsWith", types.FilterEndsWith("name", "ple"), types.NewFilter("", "name", types.FilterOpEndsWith, "ple")},
		{"IsNull", types.FilterIsNull("expired_at"), types.NewFilter("", "expired_at", types.FilterOpIsNull, true)},
		{"IsNotNull", types.FilterIsNotNull("expired_at"), types.NewFilter("", "expired_at", types.FilterOpIsNull, false)},
		{"Regex", types.FilterRegex("name", "^sam"), types.NewFilter("", "name", types.FilterOpRegex, "^sam")},
		{"NotRegex", types.FilterNotRegex("name", "^sam"), types.NewFilter("", "name", types.FilterOpNotRegex, "^sam")},
		{"JSONContains", types.FilterJSONContains("tags", "sample"), types.NewFilter("", "tags", types.FilterOpJSONContains, "sample")},
		{"False", types.FilterFalse(), types.NewFilter("", "", types.FilterOpFalse, nil)},
		{
			"Or",
			types.FilterOr(types.FilterEq("age", 18), types.FilterEq("name", "sample")),
			types.NewFilter("", "", types.FilterOpOr, []types.Filter{
				types.NewFilter("", "age", types.FilterOpEq, 18),
				types.NewFilter("", "name", types.FilterOpEq, "sample"),
			}),
		},
		{
			"And",
			types.FilterAnd(types.FilterEq("age", 18), types.FilterEq("name", "sample")),
			types.NewFilter("", "", types.FilterOpAnd, []types.Filter{
				types.NewFilter("", "age", types.FilterOpEq, 18),
				types.NewFilter("", "name", types.FilterOpEq, "sample"),
			}),
		},
		{
			"NestedGroups",
			types.FilterOr(types.FilterAnd(types.FilterEq("age", 18))),
			types.NewFilter("", "", types.FilterOpOr, []types.Filter{
				types.NewFilter("", "", types.FilterOpAnd, []types.Filter{
					types.NewFilter("", "age", types.FilterOpEq, 18),
				}),
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}

func TestFilterGroupsKeepTheirOwnFilters(t *testing.T) {
	// Two groups built from one prefix slice with spare capacity keep their
	// own members: the second append does not rewrite the first.
	base := make([]types.Filter, 0, 4)
	base = append(base, types.FilterEq("status", "done"))
	or := types.FilterOr(append(base, types.FilterEq("kind", "gold"))...)
	and := types.FilterAnd(append(base, types.FilterEq("kind", "silver"))...)
	exists := types.FilterExists[*sampleRecord](append(base, types.FilterEq("kind", "bronze"))...)
	orMembers, ok := or.Value().([]types.Filter)
	require.True(t, ok)
	andMembers, ok := and.Value().([]types.Filter)
	require.True(t, ok)
	sub, ok := exists.Value().(types.Subquery)
	require.True(t, ok)
	require.Equal(t, "gold", orMembers[1].Value())
	require.Equal(t, "silver", andMembers[1].Value())
	require.Equal(t, "bronze", sub.Filters[1].Value())
}

func TestFilterListsKeepTheirOwnValues(t *testing.T) {
	// Two lists built from one prefix slice with spare capacity keep their
	// own members: the second append does not rewrite the first.
	base := make([]string, 0, 4)
	base = append(base, "done")
	in := types.FilterIn("status", append(base, "gold"))
	notIn := types.FilterNotIn("status", append(base, "silver"))
	require.Equal(t, []string{"done", "gold"}, in.Value())
	require.Equal(t, []string{"done", "silver"}, notIn.Value())
}
