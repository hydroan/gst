package gst_test

import (
	"testing"

	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

func TestParseFilterOp(t *testing.T) {
	t.Run("AcceptsEveryKnownOperator", func(t *testing.T) {
		for s, want := range map[string]gst.FilterOp{
			"eq":         gst.FilterOpEq,
			"ne":         gst.FilterOpNe,
			"gt":         gst.FilterOpGt,
			"gte":        gst.FilterOpGte,
			"lt":         gst.FilterOpLt,
			"lte":        gst.FilterOpLte,
			"in":         gst.FilterOpIn,
			"notin":      gst.FilterOpNotIn,
			"like":       gst.FilterOpLike,
			"notlike":    gst.FilterOpNotLike,
			"ilike":      gst.FilterOpILike,
			"startswith": gst.FilterOpStartsWith,
			"endswith":   gst.FilterOpEndsWith,
			"isnull":     gst.FilterOpIsNull,
		} {
			op, ok := gst.ParseFilterOp(s)
			require.True(t, ok, "operator %q must be recognized", s)
			require.Equal(t, want, op)
		}
	})

	t.Run("RejectsUnknownOperator", func(t *testing.T) {
		for _, s := range []string{"", "eq ", "EQ", "between", "notnull"} {
			_, ok := gst.ParseFilterOp(s)
			require.False(t, ok, "operator %q must be rejected", s)
		}
	})

	t.Run("ServiceOnlyOperatorsAreNotParseable", func(t *testing.T) {
		for _, op := range []gst.FilterOp{
			gst.FilterOpRegex, gst.FilterOpNotRegex, gst.FilterOpJSONContains,
			gst.FilterOpOr, gst.FilterOpAnd, gst.FilterOpExists,
			gst.FilterOpEqCol, gst.FilterOpFalse,
		} {
			_, ok := gst.ParseFilterOp(string(op))
			require.False(t, ok, "service-only operator %q must not be reachable from URL parsing", op)
		}
	})
}
