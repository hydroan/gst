package types_test

import (
	"testing"
	"time"

	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

func TestColumnSplit(t *testing.T) {
	status := types.NewColumn[sampleTable, sampleStatus]("status")
	parsed := types.Filter{Column: "status", Op: types.FilterOpIn, Value: []string{"active"}}
	built := status.Eq(sampleStatusActive)
	otherColumn := types.Filter{Column: "amount", Op: types.FilterOpGt, Value: "1"}
	otherTable := types.NewColumn[*sampleRecord, string]("status").Eq("active")
	group := types.FilterOr(built, otherColumn)

	own, rest := status.Split([]types.Filter{parsed, otherColumn, built, otherTable, group})

	require.Equal(t, []types.Filter{parsed, built}, own,
		"a parsed filter names no table and matches by name, a built one matches by name and table, both in their order")
	require.Equal(t, []types.Filter{otherColumn, otherTable, group}, rest,
		"another column, a same-named column of another table and a group stay with the rest")
}

func TestColumnValues(t *testing.T) {
	status := types.NewColumn[sampleTable, sampleStatus]("status")

	t.Run("ReadsParsedAndBuiltEqualityFilters", func(t *testing.T) {
		values, err := status.Values([]types.Filter{
			{Column: "status", Op: types.FilterOpEq, Value: "active"},
			{Column: "amount", Op: types.FilterOpGt, Value: "1"},
			{Column: "status", Op: types.FilterOpIn, Value: []string{"removed", "active"}},
			status.In(sampleStatusRemoved),
		})
		require.NoError(t, err)
		require.Equal(t, []sampleStatus{sampleStatusActive, sampleStatusRemoved, sampleStatusActive, sampleStatusRemoved}, values,
			"filters on other columns are ignored and the values keep filter order")
	})

	t.Run("ConvertsParsedValuesToTheColumnType", func(t *testing.T) {
		// A parsed filter carries the canonical form the URL parser
		// normalized it to: the string spelling of a number, a bool for a
		// bool column.
		amounts, err := types.NewNumericColumn[sampleTable, int64]("amount").Values([]types.Filter{
			{Column: "amount", Op: types.FilterOpIn, Value: []string{"10", "-3"}},
		})
		require.NoError(t, err)
		require.Equal(t, []int64{10, -3}, amounts)

		ratios, err := types.NewNumericColumn[sampleTable, float64]("ratio").Values([]types.Filter{
			{Column: "ratio", Op: types.FilterOpEq, Value: "0.5"},
		})
		require.NoError(t, err)
		require.Equal(t, []float64{0.5}, ratios)

		flags, err := types.NewColumn[sampleTable, bool]("enabled").Values([]types.Filter{
			{Column: "enabled", Op: types.FilterOpEq, Value: true},
		})
		require.NoError(t, err)
		require.Equal(t, []bool{true}, flags)
	})

	t.Run("ConvertsNumbersThatFit", func(t *testing.T) {
		// A filter built from a plain column name carries whatever numeric
		// type its caller passed.
		counts, err := types.NewNumericColumn[sampleTable, uint8]("count").Values([]types.Filter{
			types.FilterEq("count", 7),
			types.FilterIn("count", []int64{0, 255}),
		})
		require.NoError(t, err)
		require.Equal(t, []uint8{7, 0, 255}, counts)
	})

	t.Run("RefusesOtherOperators", func(t *testing.T) {
		_, err := status.Values([]types.Filter{{Column: "status", Op: types.FilterOpLike, Value: "act"}})
		require.ErrorContains(t, err, `operator "like" is not an equality filter`)
	})

	t.Run("RefusesValuesThatDoNotConvert", func(t *testing.T) {
		amount := types.NewNumericColumn[sampleTable, int8]("amount")
		_, err := amount.Values([]types.Filter{{Column: "amount", Op: types.FilterOpEq, Value: "ten"}})
		require.ErrorContains(t, err, `column "amount"`)
		_, err = amount.Values([]types.Filter{types.FilterEq("amount", 300)})
		require.Error(t, err, "a number the column type cannot hold is refused rather than truncated")
		_, err = amount.Values([]types.Filter{{Column: "amount", Op: types.FilterOpIn, Value: "1,2"}})
		require.ErrorContains(t, err, "is not a list")
	})
}

func TestColumnBounds(t *testing.T) {
	createdAt := types.NewTimeColumn[sampleTable]("created_at")
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)

	t.Run("ReadsParsedTimeBounds", func(t *testing.T) {
		lower, upper, err := createdAt.Bounds([]types.Filter{
			{Column: "created_at", Op: types.FilterOpGte, Value: from.Format(types.FilterTimeLayout)},
			{Column: "status", Op: types.FilterOpEq, Value: "active"},
			{Column: "created_at", Op: types.FilterOpLte, Value: to.Format(types.FilterTimeLayout)},
		})
		require.NoError(t, err)
		require.Equal(t, types.Bound[time.Time]{Value: from, Inclusive: true, Present: true}, lower)
		require.Equal(t, types.Bound[time.Time]{Value: to, Inclusive: true, Present: true}, upper)
	})

	t.Run("ReadsBuiltExclusiveBounds", func(t *testing.T) {
		amount := types.NewNumericColumn[sampleTable, int64]("amount")
		lower, upper, err := amount.Bounds([]types.Filter{amount.Gt(10), amount.Lt(20)})
		require.NoError(t, err)
		require.Equal(t, types.Bound[int64]{Value: 10, Present: true}, lower)
		require.Equal(t, types.Bound[int64]{Value: 20, Present: true}, upper)
	})

	t.Run("LeavesAnAbsentEndOpen", func(t *testing.T) {
		lower, upper, err := createdAt.Bounds([]types.Filter{createdAt.Lte(to)})
		require.NoError(t, err)
		require.False(t, lower.Present)
		require.Equal(t, types.Bound[time.Time]{Value: to, Inclusive: true, Present: true}, upper)
	})

	t.Run("RefusesOtherOperators", func(t *testing.T) {
		_, _, err := createdAt.Bounds([]types.Filter{createdAt.Eq(from)})
		require.ErrorContains(t, err, `operator "eq" is not a range filter`)
	})

	t.Run("RefusesASecondFilterForOneEnd", func(t *testing.T) {
		_, _, err := createdAt.Bounds([]types.Filter{createdAt.Gt(from), createdAt.Gte(from)})
		require.ErrorContains(t, err, "more than one filter gives the lower bound")
	})

	t.Run("RefusesValuesThatDoNotConvert", func(t *testing.T) {
		_, _, err := createdAt.Bounds([]types.Filter{{Column: "created_at", Op: types.FilterOpLt, Value: "yesterday"}})
		require.ErrorContains(t, err, `column "created_at"`)
	})
}
