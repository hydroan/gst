package database_test

import (
	"context"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

// Tests for the grouped side of the select builder (select_group.go): group
// keys, measures, conditional measures, time buckets and HAVING, over the
// seeded rows described at aggregateSeed in fixture_test.go.

func TestSelectScalar(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	// MIN, MAX and AVG are NULL for a group with no matching row, so their
	// result fields are pointers; SUM is coalesced to zero and is not.
	type row struct {
		Total    int64
		Records  int64
		Smallest *int64
		Largest  *int64
		AvgScore *float64
	}
	got := row{}
	require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.Amount.Sum().As("total"),
		types.Count().As("records"),
		TestAggregateRecordCols.Amount.Min().As("smallest"),
		TestAggregateRecordCols.Amount.Max().As("largest"),
		TestAggregateRecordCols.Score.Avg().As("avg_score"),
	).
		ScanOne(&got))

	require.EqualValues(t, 2100, got.Total)
	require.EqualValues(t, 6, got.Records)
	require.NotNil(t, got.Smallest)
	require.EqualValues(t, 100, *got.Smallest)
	require.NotNil(t, got.Largest)
	require.EqualValues(t, 600, *got.Largest)
	require.NotNil(t, got.AvgScore)
	require.InDelta(t, 4.0, *got.AvgScore, 0.0001)
}

func TestSelectGroupBy(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Category string
		Total    int64
		Records  int64
	}
	rows := make([]row, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.Category.Group(),
		TestAggregateRecordCols.Amount.Sum().As("total"),
		types.Count().As("records"),
	).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Scan(&rows))

	require.Equal(t, []row{
		{Category: "alpha", Total: 600, Records: 3},
		{Category: "beta", Total: 900, Records: 2},
		{Category: "gamma", Total: 600, Records: 1},
	}, rows)
}

func TestSelectConditional(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	// One scan produces a column per status. Without conditional aggregation
	// this report costs one full scan per status column.
	type row struct {
		Category    string
		DoneAmount  int64
		FailAmount  int64
		DoneRecords int64
		// None is a measure whose condition never holds: the false predicate
		// composes into the CASE guard like any filter and counts nothing.
		None int64
	}
	rows := make([]row, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.Category.Group(),
		TestAggregateRecordCols.Amount.Sum().Where(TestAggregateRecordCols.Status.Eq("done")).As("done_amount"),
		TestAggregateRecordCols.Amount.Sum().Where(TestAggregateRecordCols.Status.Eq("failed")).As("fail_amount"),
		types.Count().Where(TestAggregateRecordCols.Status.Eq("done")).As("done_records"),
		types.Count().Where(types.FilterFalse()).As("none"),
	).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Scan(&rows))

	require.Equal(t, []row{
		{Category: "alpha", DoneAmount: 300, FailAmount: 300, DoneRecords: 2, None: 0},
		{Category: "beta", DoneAmount: 400, FailAmount: 500, DoneRecords: 1, None: 0},
		// gamma has no failed row: an empty SUM is coalesced to 0, never NULL.
		{Category: "gamma", DoneAmount: 600, FailAmount: 0, DoneRecords: 1, None: 0},
	}, rows)
}

func TestSelectHavingAndTopN(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Category string
		Total    int64
	}
	total := TestAggregateRecordCols.Amount.Sum().As("total")

	t.Run("HavingFiltersGroups", func(t *testing.T) {
		rows := make([]row, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), total).
			Having(total.Gt(600)).
			Scan(&rows))
		require.Equal(t, []row{{Category: "beta", Total: 900}}, rows)
	})

	t.Run("HavingOnConditionalMeasure", func(t *testing.T) {
		// The measure carries a predicate, so HAVING has to re-render it with
		// its value bound. Rendering it for the log instead would put the Go
		// formatting of the predicate into the SQL, which the database accepts
		// as a constant and answers with the wrong groups.
		type condRow struct {
			Category string
			Done     int64
		}
		done := types.Count().Where(TestAggregateRecordCols.Status.Eq("done")).As("done")
		rows := make([]condRow, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, condRow](context.Background(), TestAggregateRecordCols.Category.Group(), done).
			Having(done.Gt(1)).
			Scan(&rows))
		// done rows per category: alpha 2, beta 1, gamma 1.
		require.Equal(t, []condRow{{Category: "alpha", Done: 2}}, rows)
	})

	t.Run("OrderByMeasureWithLimit", func(t *testing.T) {
		// alpha and gamma tie on 600, so the group key breaks the tie: without
		// it the databases would be free to answer either group second.
		rows := make([]row, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), total).
			OrderBy(total.Desc(), TestAggregateRecordCols.Category.Group().Asc()).
			Limit(2).
			Scan(&rows))
		require.Equal(t, []row{{Category: "beta", Total: 900}, {Category: "alpha", Total: 600}}, rows)
	})

	t.Run("OffsetPagesGroups", func(t *testing.T) {
		rows := make([]row, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), total).
			OrderBy(total.Desc(), TestAggregateRecordCols.Category.Group().Asc()).
			Limit(1).
			Offset(1).
			Scan(&rows))
		require.Equal(t, []row{{Category: "alpha", Total: 600}}, rows)
	})

	// Every HAVING operator, over totals of alpha 600, beta 900, gamma 600.
	// Eq matters doubly: the renderer spells it through its default arm, so
	// only a live query proves the spelling.
	t.Run("HavingOperators", func(t *testing.T) {
		cases := []struct {
			name   string
			having types.TermCondition
			want   []row
		}{
			{"Eq", total.Eq(900), []row{{Category: "beta", Total: 900}}},
			{"Ne", total.Ne(600), []row{{Category: "beta", Total: 900}}},
			{"Gte", total.Gte(900), []row{{Category: "beta", Total: 900}}},
			{"Lt", total.Lt(700), []row{{Category: "alpha", Total: 600}, {Category: "gamma", Total: 600}}},
			{"Lte", total.Lte(600), []row{{Category: "alpha", Total: 600}, {Category: "gamma", Total: 600}}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rows := make([]row, 0)
				require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), total).
					Having(tc.having).
					OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
					Scan(&rows))
				require.Equal(t, tc.want, rows)
			})
		}
	})

	// A non-positive limit means no limit and a non-positive offset no skip,
	// matching Database.WithLimit/WithOffset so the two APIs read the same.
	t.Run("NonPositiveLimitAndOffsetReset", func(t *testing.T) {
		rows := make([]row, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), total).
			OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
			Limit(1).
			Limit(0).
			Offset(0).
			Scan(&rows))
		require.Len(t, rows, 3, "Limit(0) must clear the cap and Offset(0) must not skip")
	})
}

func TestSelectCountDistinct(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Categories int64
	}
	got := row{}
	require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.CountDistinct().As("categories")).
		ScanOne(&got))
	require.EqualValues(t, 3, got.Categories)
}

func TestSelectTimeBucket(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Bucket  string
		Total   int64
		Records int64
	}

	t.Run("ByDay", func(t *testing.T) {
		rows := make([]row, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(),
			TestAggregateRecordCols.OccurredAt.ByDay().As("bucket"),
			TestAggregateRecordCols.Amount.Sum().As("total"),
			types.Count().As("records"),
		).
			OrderBy(TestAggregateRecordCols.OccurredAt.ByDay().As("bucket").Asc()).
			Scan(&rows))

		require.Equal(t, []row{
			{Bucket: "2024-01-10", Total: 300, Records: 2},
			{Bucket: "2024-01-11", Total: 300, Records: 1},
			{Bucket: "2024-02-10", Total: 900, Records: 2},
			{Bucket: "2024-02-11", Total: 600, Records: 1},
		}, rows)
	})

	t.Run("ByMonth", func(t *testing.T) {
		rows := make([]row, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(),
			TestAggregateRecordCols.OccurredAt.ByMonth().As("bucket"),
			TestAggregateRecordCols.Amount.Sum().As("total"),
			types.Count().As("records"),
		).
			OrderBy(TestAggregateRecordCols.OccurredAt.ByMonth().As("bucket").Asc()).
			Scan(&rows))

		require.Equal(t, []row{
			{Bucket: "2024-01", Total: 600, Records: 3},
			{Bucket: "2024-02", Total: 1500, Records: 3},
		}, rows)
	})

	t.Run("ByHour", func(t *testing.T) {
		rows := make([]row, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(),
			TestAggregateRecordCols.OccurredAt.ByHour().As("bucket"),
			TestAggregateRecordCols.Amount.Sum().As("total"),
			types.Count().As("records"),
		).
			Where(TestAggregateRecordCols.Category.Eq("alpha")).
			OrderBy(TestAggregateRecordCols.OccurredAt.ByHour().As("bucket").Asc()).
			Scan(&rows))

		require.Equal(t, []row{
			{Bucket: "2024-01-10 08:00:00", Total: 100, Records: 1},
			{Bucket: "2024-01-10 09:00:00", Total: 200, Records: 1},
			{Bucket: "2024-01-11 08:00:00", Total: 300, Records: 1},
		}, rows)
	})
}

// TestSumRejectsTextBackedValuer pins SUM to the single classification rule.
// Admitting any struct that stores itself through driver.Valuer would take in
// gorm.DeletedAt, which every model carries, and the null wrappers -- the exact
// types ClassifyColumn exists to keep out.
func TestSumRejectsTextBackedValuer(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	rows := make([]struct{ Total int64 }, 0)
	require.ErrorIs(t, database.Select[*TestAggregateRecord, struct{ Total int64 }](context.Background(), types.NewNumericColumn[*TestAggregateRecord, int64]("deleted_at").Sum().As("total")).
		Scan(&rows), database.ErrAggregateType)
}

// TestSelectHavingValue pins the values a post-aggregation comparison
// accepts. nil renders as a comparison against NULL, which no group satisfies,
// so a report would come back empty with no sign of the mistake.
func TestSelectHavingValue(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	ctx := context.Background()
	type row struct {
		Category string
		Total    int64
	}
	total := TestAggregateRecordCols.Amount.Sum().As("total")

	for name, value := range map[string]any{
		"Nil":              nil,
		"Slice":            []int64{1, 2},
		"TypedNilPointer":  (*int64)(nil),
		"NestedNilPointer": new(*int64),
	} {
		t.Run(name, func(t *testing.T) {
			rows := make([]row, 0)
			require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), total).
				Having(total.Gt(value)).
				Scan(&rows), database.ErrHavingValue)
		})
	}
}

// TestSelectGroupByRendersRawExpression pins that the group key reaches gorm
// as an already-quoted expression it must not quote again. The MySQL, PostgreSQL
// and SQLite quoters are idempotent so a double quote is invisible there;
// the ClickHouse one is not, and would emit ""col"".
func TestSelectGroupByRendersRawExpression(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Category string
		Total    int64
	}
	statements := make([]types.SQLStatement, 0)
	rows := make([]row, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
		WithDryRun(&statements).
		Scan(&rows))

	require.Len(t, statements, 1)
	require.Contains(t, statements[0].Query, "GROUP BY "+quoteIdent("category"))
	require.NotContains(t, statements[0].Query, quoteIdent(quoteIdent("category")))
}
