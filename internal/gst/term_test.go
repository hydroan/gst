package gst_test

import (
	"testing"

	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

func TestTermFnValid(t *testing.T) {
	for _, fn := range []gst.TermFn{
		gst.FnNone, gst.FnCount, gst.FnCountDistinct,
		gst.FnSum, gst.FnAvg, gst.FnMin, gst.FnMax,
		gst.FnRowNumber, gst.FnRank, gst.FnDenseRank, gst.FnLag, gst.FnLead,
	} {
		require.True(t, fn.Valid(), "function %q must be valid", fn)
	}
	require.False(t, gst.TermFn("MEDIAN").Valid(), "a function outside the set must be rejected")
}

func TestTimeBucketValid(t *testing.T) {
	for _, bucket := range []gst.TimeBucket{
		gst.TimeBucketNone, gst.TimeBucketHour, gst.TimeBucketDay, gst.TimeBucketMonth,
	} {
		require.True(t, bucket.Valid(), "bucket %q must be valid", bucket)
	}
	require.False(t, gst.TimeBucket("week").Valid(), "a bucket outside the set must be rejected")
}

func TestColumnlessTermsCarryDefaultAliases(t *testing.T) {
	// A term without a column has no column name to project under, so each
	// carries an alias of its own until As renames it.
	require.Equal(t, gst.NewTerm(gst.FnCount, "", "", "", gst.DefaultCountAlias), gst.Count())
	require.Equal(t, gst.NewTerm(gst.FnRowNumber, "", "", "", "row_number"), gst.RowNumber())
	require.Equal(t, gst.NewTerm(gst.FnRank, "", "", "", "rank"), gst.Rank())
	require.Equal(t, gst.NewTerm(gst.FnDenseRank, "", "", "", "dense_rank"), gst.DenseRank())
}

func TestTermKinds(t *testing.T) {
	category := gst.NewColumn[sampleTable, string]("category")

	tests := []struct {
		label    string
		term     gst.Term
		measure  bool
		groupKey bool
		plain    bool
	}{
		{"GroupKey", category.Group(), false, true, false},
		{"PlainColumn", gst.TermOf(category), false, false, true},
		{"Aggregate", category.Count(), true, false, false},
		{"WindowFunction", gst.RowNumber(), true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			require.Equal(t, tt.measure, gst.TermIsMeasure(tt.term))
			require.Equal(t, tt.groupKey, gst.TermIsGroupKey(tt.term))
			require.Equal(t, tt.plain, gst.TermIsPlain(tt.term))
			require.False(t, gst.TermIsWindowed(tt.term), "no term here declares a window")
		})
	}
}

func TestTermOf(t *testing.T) {
	category := gst.NewColumn[sampleTable, string]("category")

	t.Run("ColumnProjectsAsStored", func(t *testing.T) {
		require.Equal(t,
			gst.PlainTerm("samples", "category").As("category"),
			gst.TermOf(category))
	})

	t.Run("TermPassesThrough", func(t *testing.T) {
		total := gst.NewNumericColumn[sampleTable, int64]("amount").Sum().As("total")
		require.Equal(t, total, gst.TermOf(total))
	})
}

func TestTermModifiersReturnNewTerms(t *testing.T) {
	total := gst.NewNumericColumn[sampleTable, int64]("amount").Sum()

	t.Run("As", func(t *testing.T) {
		require.Equal(t, "total", gst.TermAliasOf(total.As("total")))
		require.Equal(t, "amount", gst.TermAliasOf(total), "the original term keeps its column as alias")
	})

	t.Run("Over", func(t *testing.T) {
		window := gst.PartitionBy(gst.NewColumn[sampleTable, string]("tenant_id"))
		windowed := total.Over(window)
		require.Equal(t, &window, gst.TermWindowOf(windowed))
		require.True(t, gst.TermIsWindowed(windowed))
		require.False(t, gst.TermIsWindowed(total), "the original term stays unwindowed")
	})
}

func TestTermAsEmptyKeepsTheAlias(t *testing.T) {
	// An empty alias is not an alias: the term stays as it was, under its
	// default or under the alias it already had.
	amount := gst.NewNumericColumn[sampleTable, int64]("amount")
	require.Equal(t, amount.Sum(), amount.Sum().As(""))
	require.Equal(t, amount.Sum().As("total"), amount.Sum().As("total").As(""))
	require.Equal(t, gst.Count(), gst.Count().As(""))
	require.Equal(t, amount.As("total"), amount.As("total").As(""))
	require.Equal(t, "amount", gst.TermAliasOf(amount.As("")))
}
