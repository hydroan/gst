package types_test

import (
	"testing"

	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

func TestCompareOpValid(t *testing.T) {
	for _, op := range []types.CompareOp{
		types.CompareEq, types.CompareNe, types.CompareGt, types.CompareGte, types.CompareLt, types.CompareLte,
	} {
		require.True(t, op.Valid(), "comparison %q must be valid", op)
	}
	require.False(t, types.CompareOp("like").Valid(), "a comparison outside the set must be rejected")
	require.False(t, types.CompareOp("").Valid(), "the zero comparison is no comparison at all")
}

func TestTermBuildsConditions(t *testing.T) {
	total := types.NewNumericColumn[sampleTable, int64]("amount").Sum()

	tests := []struct {
		label string
		got   types.TermCondition
		want  types.TermCondition
	}{
		{"Eq", total.Eq(10), types.NewTermCondition(total, types.CompareEq, 10)},
		{"Ne", total.Ne(10), types.NewTermCondition(total, types.CompareNe, 10)},
		{"Gt", total.Gt(10), types.NewTermCondition(total, types.CompareGt, 10)},
		{"Gte", total.Gte(10), types.NewTermCondition(total, types.CompareGte, 10)},
		{"Lt", total.Lt(10), types.NewTermCondition(total, types.CompareLt, 10)},
		{"Lte", total.Lte(10), types.NewTermCondition(total, types.CompareLte, 10)},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}
