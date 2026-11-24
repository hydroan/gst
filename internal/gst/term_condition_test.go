package gst_test

import (
	"testing"

	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

func TestCompareOpValid(t *testing.T) {
	for _, op := range []gst.CompareOp{
		gst.CompareEq, gst.CompareNe, gst.CompareGt, gst.CompareGte, gst.CompareLt, gst.CompareLte,
	} {
		require.True(t, op.Valid(), "comparison %q must be valid", op)
	}
	require.False(t, gst.CompareOp("like").Valid(), "a comparison outside the set must be rejected")
	require.False(t, gst.CompareOp("").Valid(), "the zero comparison is no comparison at all")
}

func TestTermBuildsConditions(t *testing.T) {
	total := gst.NewNumericColumn[sampleTable, int64]("amount").Sum()

	tests := []struct {
		label string
		got   gst.TermCondition
		want  gst.TermCondition
	}{
		{"Eq", total.Eq(10), gst.NewTermCondition(total, gst.CompareEq, 10)},
		{"Ne", total.Ne(10), gst.NewTermCondition(total, gst.CompareNe, 10)},
		{"Gt", total.Gt(10), gst.NewTermCondition(total, gst.CompareGt, 10)},
		{"Gte", total.Gte(10), gst.NewTermCondition(total, gst.CompareGte, 10)},
		{"Lt", total.Lt(10), gst.NewTermCondition(total, gst.CompareLt, 10)},
		{"Lte", total.Lte(10), gst.NewTermCondition(total, gst.CompareLte, 10)},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}
