package gst_test

import (
	"testing"
	"time"

	"github.com/hydroan/gst/internal/datatypes"
	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

func TestColumnSplit(t *testing.T) {
	status := gst.NewColumn[sampleTable, sampleStatus]("status")
	parsed := gst.NewFilter("", "status", gst.FilterOpIn, []string{"active"})
	built := status.Eq(sampleStatusActive)
	otherColumn := gst.NewFilter("", "amount", gst.FilterOpGt, "1")
	otherTable := gst.NewColumn[*sampleRecord, string]("status").Eq("active")
	group := gst.FilterOr(built, otherColumn)

	own, rest := status.Split([]gst.Filter{parsed, otherColumn, built, otherTable, group})

	require.Equal(t, []gst.Filter{parsed, built}, own,
		"a parsed filter names no table and matches by name, a built one matches by name and table, both in their order")
	require.Equal(t, []gst.Filter{otherColumn, otherTable, group}, rest,
		"another column, a same-named column of another table and a group stay with the rest")
}

func TestColumnValues(t *testing.T) {
	status := gst.NewColumn[sampleTable, sampleStatus]("status")

	t.Run("ReadsParsedAndBuiltEqualityFilters", func(t *testing.T) {
		values, err := status.Values([]gst.Filter{
			gst.NewFilter("", "status", gst.FilterOpEq, "active"),
			gst.NewFilter("", "amount", gst.FilterOpGt, "1"),
			gst.NewFilter("", "status", gst.FilterOpIn, []string{"removed", "active"}),
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
		amounts, err := gst.NewNumericColumn[sampleTable, int64]("amount").Values([]gst.Filter{
			gst.NewFilter("", "amount", gst.FilterOpIn, []string{"10", "-3"}),
		})
		require.NoError(t, err)
		require.Equal(t, []int64{10, -3}, amounts)

		ratios, err := gst.NewNumericColumn[sampleTable, float64]("ratio").Values([]gst.Filter{
			gst.NewFilter("", "ratio", gst.FilterOpEq, "0.5"),
		})
		require.NoError(t, err)
		require.Equal(t, []float64{0.5}, ratios)

		flags, err := gst.NewColumn[sampleTable, bool]("enabled").Values([]gst.Filter{
			gst.NewFilter("", "enabled", gst.FilterOpEq, true),
		})
		require.NoError(t, err)
		require.Equal(t, []bool{true}, flags)
	})

	t.Run("ConvertsNumbersThatFit", func(t *testing.T) {
		// A filter built from a plain column name carries whatever numeric
		// type its caller passed.
		counts, err := gst.NewNumericColumn[sampleTable, uint8]("count").Values([]gst.Filter{
			gst.FilterEq("count", 7),
			gst.FilterIn("count", []int64{0, 255}),
		})
		require.NoError(t, err)
		require.Equal(t, []uint8{7, 0, 255}, counts)
	})

	t.Run("RefusesOtherOperators", func(t *testing.T) {
		_, err := status.Values([]gst.Filter{gst.NewFilter("", "status", gst.FilterOpLike, "act")})
		require.ErrorContains(t, err, `operator "like" is not an equality filter`)
	})

	t.Run("RefusesValuesThatDoNotConvert", func(t *testing.T) {
		amount := gst.NewNumericColumn[sampleTable, int8]("amount")
		_, err := amount.Values([]gst.Filter{gst.NewFilter("", "amount", gst.FilterOpEq, "ten")})
		require.ErrorContains(t, err, `column "amount"`)
		_, err = amount.Values([]gst.Filter{gst.FilterEq("amount", 300)})
		require.Error(t, err, "a number the column type cannot hold is refused rather than truncated")
		_, err = amount.Values([]gst.Filter{gst.NewFilter("", "amount", gst.FilterOpIn, "1,2")})
		require.ErrorContains(t, err, "is not a list")
	})
}

func TestColumnExcludedValues(t *testing.T) {
	status := gst.NewColumn[sampleTable, sampleStatus]("status")

	t.Run("ReadsParsedAndBuiltExclusionFilters", func(t *testing.T) {
		values, err := status.ExcludedValues([]gst.Filter{
			gst.NewFilter("", "status", gst.FilterOpNe, "active"),
			gst.NewFilter("", "amount", gst.FilterOpGt, "1"),
			gst.NewFilter("", "status", gst.FilterOpNotIn, []string{"removed", "active"}),
			status.NotIn(sampleStatusRemoved),
		})
		require.NoError(t, err)
		require.Equal(t, []sampleStatus{sampleStatusActive, sampleStatusRemoved, sampleStatusActive, sampleStatusRemoved}, values,
			"filters on other columns are ignored and the values keep filter order")
	})

	t.Run("ConvertsParsedValuesToTheColumnType", func(t *testing.T) {
		// A parsed filter carries the canonical form the URL parser
		// normalized it to: the UTC wall clock in FilterTimeLayout for a time,
		// the string spellings of a number for the members of a notin.
		at := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
		times, err := gst.NewTimeColumn[sampleTable]("created_at").ExcludedValues([]gst.Filter{
			gst.NewFilter("", "created_at", gst.FilterOpNe, at.Format(gst.FilterTimeLayout)),
		})
		require.NoError(t, err)
		require.Equal(t, []time.Time{at}, times)

		amounts, err := gst.NewNumericColumn[sampleTable, int64]("amount").ExcludedValues([]gst.Filter{
			gst.NewFilter("", "amount", gst.FilterOpNotIn, []string{"10", "-3"}),
		})
		require.NoError(t, err)
		require.Equal(t, []int64{10, -3}, amounts)
	})

	t.Run("RefusesOtherOperators", func(t *testing.T) {
		_, err := status.ExcludedValues([]gst.Filter{status.Eq(sampleStatusActive)})
		require.ErrorContains(t, err, `operator "eq" is not an exclusion filter`)
	})

	t.Run("RefusesValuesThatDoNotConvert", func(t *testing.T) {
		amount := gst.NewNumericColumn[sampleTable, int8]("amount")
		_, err := amount.ExcludedValues([]gst.Filter{gst.NewFilter("", "amount", gst.FilterOpNe, "ten")})
		require.ErrorContains(t, err, `column "amount"`)
		_, err = amount.ExcludedValues([]gst.Filter{gst.NewFilter("", "amount", gst.FilterOpNotIn, "1,2")})
		require.ErrorContains(t, err, "is not a list")
	})
}

func TestColumnBounds(t *testing.T) {
	createdAt := gst.NewTimeColumn[sampleTable]("created_at")
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)

	t.Run("ReadsParsedTimeBounds", func(t *testing.T) {
		lower, upper, err := createdAt.Bounds([]gst.Filter{
			gst.NewFilter("", "created_at", gst.FilterOpGte, from.Format(gst.FilterTimeLayout)),
			gst.NewFilter("", "status", gst.FilterOpEq, "active"),
			gst.NewFilter("", "created_at", gst.FilterOpLte, to.Format(gst.FilterTimeLayout)),
		})
		require.NoError(t, err)
		require.Equal(t, gst.Bound[time.Time]{Value: from, Inclusive: true, Present: true}, lower)
		require.Equal(t, gst.Bound[time.Time]{Value: to, Inclusive: true, Present: true}, upper)
	})

	t.Run("ReadsParsedDayBounds", func(t *testing.T) {
		// A date column's filter parsed from a request carries the day's
		// midnight in FilterTimeLayout, and one built through the reference
		// the Date itself; both come back as the day.
		day := gst.NewDateColumn[sampleTable]("day")
		first := datatypes.Date(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
		last := datatypes.Date(time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC))
		lower, upper, err := day.Bounds([]gst.Filter{
			gst.NewFilter("", "day", gst.FilterOpGte, "2026-09-01 00:00:00"),
			day.Lt(last),
		})
		require.NoError(t, err)
		require.Equal(t, gst.Bound[datatypes.Date]{Value: first, Inclusive: true, Present: true}, lower)
		require.Equal(t, gst.Bound[datatypes.Date]{Value: last, Present: true}, upper)

		_, _, err = day.Bounds([]gst.Filter{gst.NewFilter("", "day", gst.FilterOpGte, "2026-09-01")})
		require.Error(t, err, "a value outside the canonical layout does not convert")
	})

	t.Run("ReadsBuiltExclusiveBounds", func(t *testing.T) {
		amount := gst.NewNumericColumn[sampleTable, int64]("amount")
		lower, upper, err := amount.Bounds([]gst.Filter{amount.Gt(10), amount.Lt(20)})
		require.NoError(t, err)
		require.Equal(t, gst.Bound[int64]{Value: 10, Present: true}, lower)
		require.Equal(t, gst.Bound[int64]{Value: 20, Present: true}, upper)
	})

	t.Run("LeavesAnAbsentEndOpen", func(t *testing.T) {
		lower, upper, err := createdAt.Bounds([]gst.Filter{createdAt.Lte(to)})
		require.NoError(t, err)
		require.False(t, lower.Present)
		require.Equal(t, gst.Bound[time.Time]{Value: to, Inclusive: true, Present: true}, upper)
	})

	t.Run("RefusesOtherOperators", func(t *testing.T) {
		_, _, err := createdAt.Bounds([]gst.Filter{createdAt.Eq(from)})
		require.ErrorContains(t, err, `operator "eq" is not a range filter`)
	})

	t.Run("RefusesASecondFilterForOneEnd", func(t *testing.T) {
		_, _, err := createdAt.Bounds([]gst.Filter{createdAt.Gt(from), createdAt.Gte(from)})
		require.ErrorContains(t, err, "more than one filter gives the lower bound")
	})

	t.Run("RefusesValuesThatDoNotConvert", func(t *testing.T) {
		_, _, err := createdAt.Bounds([]gst.Filter{gst.NewFilter("", "created_at", gst.FilterOpLt, "yesterday")})
		require.ErrorContains(t, err, `column "created_at"`)
	})
}
