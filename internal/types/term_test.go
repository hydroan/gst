package types_test

import (
	"testing"

	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

func TestTermFnValid(t *testing.T) {
	for _, fn := range []types.TermFn{
		types.FnNone, types.FnCount, types.FnCountDistinct,
		types.FnSum, types.FnAvg, types.FnMin, types.FnMax,
		types.FnRowNumber, types.FnRank, types.FnDenseRank, types.FnLag, types.FnLead,
	} {
		require.True(t, fn.Valid(), "function %q must be valid", fn)
	}
	require.False(t, types.TermFn("MEDIAN").Valid(), "a function outside the set must be rejected")
}

func TestTimeBucketValid(t *testing.T) {
	for _, bucket := range []types.TimeBucket{
		types.TimeBucketNone, types.TimeBucketHour, types.TimeBucketDay, types.TimeBucketMonth,
	} {
		require.True(t, bucket.Valid(), "bucket %q must be valid", bucket)
	}
	require.False(t, types.TimeBucket("week").Valid(), "a bucket outside the set must be rejected")
}

func TestColumnlessTermsCarryDefaultAliases(t *testing.T) {
	// A term without a column has no column name to project under, so each
	// carries an alias of its own until As renames it.
	require.Equal(t, types.NewTerm(types.FnCount, "", "", "", types.DefaultCountAlias), types.Count())
	require.Equal(t, types.NewTerm(types.FnRowNumber, "", "", "", "row_number"), types.RowNumber())
	require.Equal(t, types.NewTerm(types.FnRank, "", "", "", "rank"), types.Rank())
	require.Equal(t, types.NewTerm(types.FnDenseRank, "", "", "", "dense_rank"), types.DenseRank())
}

func TestTermKinds(t *testing.T) {
	category := types.NewColumn[sampleTable, string]("category")

	tests := []struct {
		label    string
		term     types.Term
		measure  bool
		groupKey bool
		plain    bool
	}{
		{"GroupKey", category.Group(), false, true, false},
		{"PlainColumn", types.TermOf(category), false, false, true},
		{"Aggregate", category.Count(), true, false, false},
		{"WindowFunction", types.RowNumber(), true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			require.Equal(t, tt.measure, tt.term.IsMeasure())
			require.Equal(t, tt.groupKey, tt.term.IsGroupKey())
			require.Equal(t, tt.plain, tt.term.IsPlain())
			require.False(t, tt.term.IsWindowed(), "no term here declares a window")
		})
	}
}

func TestTermOf(t *testing.T) {
	category := types.NewColumn[sampleTable, string]("category")

	t.Run("ColumnProjectsAsStored", func(t *testing.T) {
		require.Equal(t,
			types.PlainTerm("samples", "category").As("category"),
			types.TermOf(category))
	})

	t.Run("TermPassesThrough", func(t *testing.T) {
		total := types.NewNumericColumn[sampleTable, int64]("amount").Sum().As("total")
		require.Equal(t, total, types.TermOf(total))
	})
}

func TestTermModifiersReturnNewTerms(t *testing.T) {
	total := types.NewNumericColumn[sampleTable, int64]("amount").Sum()

	t.Run("As", func(t *testing.T) {
		require.Equal(t, "total", types.TermAliasOf(total.As("total")))
		require.Equal(t, "amount", types.TermAliasOf(total), "the original term keeps its column as alias")
	})

	t.Run("Over", func(t *testing.T) {
		window := types.PartitionBy(types.NewColumn[sampleTable, string]("tenant_id"))
		windowed := total.Over(window)
		require.Equal(t, &window, types.TermWindowOf(windowed))
		require.True(t, windowed.IsWindowed())
		require.False(t, total.IsWindowed(), "the original term stays unwindowed")
	})
}

func TestTermAsEmptyKeepsTheAlias(t *testing.T) {
	// An empty alias is not an alias: the term stays as it was, under its
	// default or under the alias it already had.
	amount := types.NewNumericColumn[sampleTable, int64]("amount")
	require.Equal(t, amount.Sum(), amount.Sum().As(""))
	require.Equal(t, amount.Sum().As("total"), amount.Sum().As("total").As(""))
	require.Equal(t, types.Count(), types.Count().As(""))
	require.Equal(t, amount.As("total"), amount.As("total").As(""))
	require.Equal(t, "amount", types.TermAliasOf(amount.As("")))
}
