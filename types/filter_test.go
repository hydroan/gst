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
