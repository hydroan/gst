package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

// The window tests read the same six seeded rows as the aggregate tests (see
// aggregateSeed in fixture_test.go), so every expectation below is a literal
// a reader can check against that table by hand.

// latestRow is the row-level projection the latest-per-group idiom reads.
type latestRow struct {
	ID       string
	Category string
	Amount   int64
	Rn       int64
}

// latestPerCategory is the latest-per-group idiom: number each category's rows
// newest first and keep the first. Two beta rows share a time, and the row
// number is stable across runs only because the framework completes the
// window's order with the primary key.
func latestPerCategory(ctx context.Context) (types.Term, types.Selector[*TestAggregateRecord, latestRow]) {
	rn := types.RowNumber().
		Over(types.PartitionBy(TestAggregateRecordCols.Category).OrderBy(TestAggregateRecordCols.OccurredAt.Desc())).
		As("rn")
	return rn, database.Select[*TestAggregateRecord, latestRow](ctx,
		TestAggregateRecordCols.ID, TestAggregateRecordCols.Category, TestAggregateRecordCols.Amount, rn).
		Qualify(rn.Eq(1)).
		OrderBy(TestAggregateRecordCols.Category.Asc())
}

func TestSelectWindowLatestPerGroup(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)
	ctx := context.Background()

	t.Run("KeepsTheFirstRowOfEachPartition", func(t *testing.T) {
		_, sel := latestPerCategory(ctx)
		rows := make([]latestRow, 0)
		require.NoError(t, sel.Scan(&rows))
		require.Equal(t, []latestRow{
			{ID: "a3", Category: "alpha", Amount: 300, Rn: 1},
			// a4 and a5 occurred at the same time; the primary key breaks
			// the tie, so a4 numbers first on every run.
			{ID: "a4", Category: "beta", Amount: 400, Rn: 1},
			{ID: "a6", Category: "gamma", Amount: 600, Rn: 1},
		}, rows)
	})

	t.Run("RendersTheWindowAndWrapsTheQualify", func(t *testing.T) {
		_, sel := latestPerCategory(ctx)
		statements := make([]types.SQLStatement, 0)
		rows := make([]latestRow, 0)
		require.NoError(t, sel.WithDryRun(&statements).Scan(&rows))
		require.Len(t, statements, 1)
		sql := statements[0].Query
		require.Contains(t, sql, "ROW_NUMBER() OVER (PARTITION BY "+quoteIdent("category")+
			" ORDER BY "+quoteIdent("occurred_at")+" DESC, "+quoteIdent("id")+" ASC) AS "+quoteIdent("rn"),
			"the order is completed with the primary key as the tie breaker")
		require.NotContains(t, sql, "ROWS BETWEEN", "a ranking function takes no frame")
		require.Contains(t, sql, ") AS q WHERE "+quoteIdent("q")+"."+quoteIdent("rn")+" = ",
			"Qualify filters a derived table, where the window column exists")
		require.Contains(t, sql, "ORDER BY "+quoteIdent("category")+" ASC", "ordering applies outside the wrap")
		require.NotContains(t, sql, "GROUP BY", "a row-level select groups nothing")
	})

	t.Run("CountsTheQualifiedRows", func(t *testing.T) {
		_, sel := latestPerCategory(ctx)
		total := 0
		require.NoError(t, sel.Count(&total))
		require.Equal(t, 3, total)

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, sel.WithDryRun(&statements).Count(&total))
		require.Len(t, statements, 1)
		require.Contains(t, statements[0].Query, "SELECT count(*) FROM (SELECT * FROM (SELECT ")
		require.Contains(t, statements[0].Query, ") AS q WHERE "+quoteIdent("q")+"."+quoteIdent("rn")+" = ")
		require.Contains(t, statements[0].Query, ") AS grouped")
	})

	t.Run("CountsEveryRowWithoutQualify", func(t *testing.T) {
		rn := types.RowNumber().Over(types.OrderBy(TestAggregateRecordCols.OccurredAt.Asc())).As("rn")
		sel := database.Select[*TestAggregateRecord, latestRow](ctx,
			TestAggregateRecordCols.ID, TestAggregateRecordCols.Category, TestAggregateRecordCols.Amount, rn)
		total := 0
		require.NoError(t, sel.Count(&total))
		require.Equal(t, 6, total)

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, sel.WithDryRun(&statements).Count(&total))
		require.Len(t, statements, 1)
		require.Contains(t, statements[0].Query, "SELECT 1 AS "+quoteIdent("row_marker"),
			"a row-level count projects a constant instead of computing the windows")
	})
}

func TestSelectWindowRunningTotal(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		ID      string
		Amount  int64
		Running int64
	}
	running := TestAggregateRecordCols.Amount.Sum().
		Over(types.PartitionBy(TestAggregateRecordCols.Category).OrderBy(TestAggregateRecordCols.OccurredAt.Asc())).
		As("running")
	statements := make([]types.SQLStatement, 0)
	rows := make([]row, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.ID, TestAggregateRecordCols.Amount, running).
		OrderBy(TestAggregateRecordCols.ID.Asc()).
		Scan(&rows))
	require.Equal(t, []row{
		{ID: "a1", Amount: 100, Running: 100},
		{ID: "a2", Amount: 200, Running: 300},
		{ID: "a3", Amount: 300, Running: 600},
		// a4 and a5 share a time: the ROWS frame with the primary key tie
		// breaker steps through them instead of showing both the total.
		{ID: "a4", Amount: 400, Running: 400},
		{ID: "a5", Amount: 500, Running: 900},
		{ID: "a6", Amount: 600, Running: 600},
	}, rows)

	require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.ID, TestAggregateRecordCols.Amount, running).
		WithDryRun(&statements).Scan(&rows))
	require.Len(t, statements, 1)
	require.Contains(t, statements[0].Query,
		"COALESCE(SUM("+quoteIdent("amount")+") OVER (PARTITION BY "+quoteIdent("category")+
			" ORDER BY "+quoteIdent("occurred_at")+" ASC, "+quoteIdent("id")+" ASC ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW), 0) AS "+quoteIdent("running"),
		"an ordered aggregate takes the ROWS frame and keeps SUM's COALESCE around the whole window")
}

func TestSelectWindowPartitionAggregates(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	// Without an order the window is the whole partition, so every row of a
	// category carries the same figures; AVG over a non-nullable column has at
	// least the current row to read and needs no pointer.
	type row struct {
		ID      string
		Rows    int64
		Total   int64
		Average float64
		Largest int64
	}
	partition := types.PartitionBy(TestAggregateRecordCols.Category)
	rows := make([]row, 0)
	statements := make([]types.SQLStatement, 0)
	sel := func() types.Selector[*TestAggregateRecord, row] {
		return database.Select[*TestAggregateRecord, row](context.Background(),
			TestAggregateRecordCols.ID,
			types.Count().Over(partition).As("rows"),
			TestAggregateRecordCols.Amount.Sum().Over(partition).As("total"),
			TestAggregateRecordCols.Amount.Avg().Over(partition).As("average"),
			TestAggregateRecordCols.Amount.Max().Over(partition).As("largest"),
		).OrderBy(TestAggregateRecordCols.ID.Asc())
	}
	require.NoError(t, sel().Scan(&rows))
	require.Equal(t, []row{
		{ID: "a1", Rows: 3, Total: 600, Average: 200, Largest: 300},
		{ID: "a2", Rows: 3, Total: 600, Average: 200, Largest: 300},
		{ID: "a3", Rows: 3, Total: 600, Average: 200, Largest: 300},
		{ID: "a4", Rows: 2, Total: 900, Average: 450, Largest: 500},
		{ID: "a5", Rows: 2, Total: 900, Average: 450, Largest: 500},
		{ID: "a6", Rows: 1, Total: 600, Average: 600, Largest: 600},
	}, rows)

	require.NoError(t, sel().WithDryRun(&statements).Scan(&rows))
	require.Len(t, statements, 1)
	require.Contains(t, statements[0].Query, "COUNT(*) OVER (PARTITION BY "+quoteIdent("category")+") AS "+quoteIdent("rows"))
	require.NotContains(t, statements[0].Query, "ROWS BETWEEN", "an unordered window takes no frame")
}

func TestSelectWindowLagLead(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		ID       string
		Amount   int64
		Previous *int64
		Next     *int64
	}
	window := types.PartitionBy(TestAggregateRecordCols.Category).OrderBy(TestAggregateRecordCols.OccurredAt.Asc())
	rows := make([]row, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.ID, TestAggregateRecordCols.Amount,
		TestAggregateRecordCols.Amount.Lag().Over(window).As("previous"),
		TestAggregateRecordCols.Amount.Lead().Over(window).As("next"),
	).OrderBy(TestAggregateRecordCols.ID.Asc()).Scan(&rows))
	amount := func(v int64) *int64 { return &v }
	require.Equal(t, []row{
		{ID: "a1", Amount: 100, Previous: nil, Next: amount(200)},
		{ID: "a2", Amount: 200, Previous: amount(100), Next: amount(300)},
		{ID: "a3", Amount: 300, Previous: amount(200), Next: nil},
		{ID: "a4", Amount: 400, Previous: nil, Next: amount(500)},
		{ID: "a5", Amount: 500, Previous: amount(400), Next: nil},
		{ID: "a6", Amount: 600, Previous: nil, Next: nil},
	}, rows)
}

func TestSelectWindowRanksGroups(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	// A window over a grouped projection reads the groups: the ranking
	// orders by the measure's full expression, and the row number breaks the
	// alpha/gamma tie with the group key.
	type row struct {
		Category  string
		Total     int64
		Rank      int64
		DenseRank int64
		RowNumber int64
	}
	total := TestAggregateRecordCols.Amount.Sum().As("total")
	byTotal := types.OrderBy(total.Desc())
	rows := make([]row, 0)
	statements := make([]types.SQLStatement, 0)
	sel := func() types.Selector[*TestAggregateRecord, row] {
		return database.Select[*TestAggregateRecord, row](context.Background(),
			TestAggregateRecordCols.Category.Group(), total,
			types.Rank().Over(byTotal),
			types.DenseRank().Over(byTotal),
			types.RowNumber().Over(byTotal),
		).OrderBy(types.RowNumber().Over(byTotal).Asc())
	}
	require.NoError(t, sel().Scan(&rows))
	require.Equal(t, []row{
		{Category: "beta", Total: 900, Rank: 1, DenseRank: 1, RowNumber: 1},
		{Category: "alpha", Total: 600, Rank: 2, DenseRank: 2, RowNumber: 2},
		{Category: "gamma", Total: 600, Rank: 2, DenseRank: 2, RowNumber: 3},
	}, rows)

	require.NoError(t, sel().WithDryRun(&statements).Scan(&rows))
	require.Len(t, statements, 1)
	require.Contains(t, statements[0].Query,
		"RANK() OVER (ORDER BY COALESCE(SUM("+quoteIdent("amount")+"), 0) DESC) AS "+quoteIdent("rank"),
		"a grouped window orders by the measure's expression; RANK keeps its peers and takes no tie breaker")
	require.Contains(t, statements[0].Query,
		"ROW_NUMBER() OVER (ORDER BY COALESCE(SUM("+quoteIdent("amount")+"), 0) DESC, "+quoteIdent("category")+" ASC) AS "+quoteIdent("row_number"),
		"ROW_NUMBER breaks the tie with the group key")
	require.Contains(t, statements[0].Query, "GROUP BY "+quoteIdent("category"))
}

func TestSelectWindowPartitionsGroupsByKey(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	// The partition key of a grouped window is one of the group keys and
	// renders as the same expression GROUP BY uses.
	type row struct {
		Category      string
		Status        string
		Total         int64
		CategoryTotal int64
	}
	rows := make([]row, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Status.Group(),
		TestAggregateRecordCols.Amount.Sum().As("total"),
		TestAggregateRecordCols.Amount.Sum().Over(types.PartitionBy(TestAggregateRecordCols.Category)).As("category_total"),
	).OrderBy(TestAggregateRecordCols.Category.Asc(), TestAggregateRecordCols.Status.Asc()).Scan(&rows))
	require.Equal(t, []row{
		{Category: "alpha", Status: "done", Total: 300, CategoryTotal: 600},
		{Category: "alpha", Status: "failed", Total: 300, CategoryTotal: 600},
		{Category: "beta", Status: "done", Total: 400, CategoryTotal: 900},
		{Category: "beta", Status: "failed", Total: 500, CategoryTotal: 900},
		{Category: "gamma", Status: "done", Total: 600, CategoryTotal: 600},
	}, rows)

	statements := make([]types.SQLStatement, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Status.Group(),
		TestAggregateRecordCols.Amount.Sum().As("total"),
		TestAggregateRecordCols.Amount.Sum().Over(types.PartitionBy(TestAggregateRecordCols.Category)).As("category_total"),
	).WithDryRun(&statements).Scan(&rows))
	require.Len(t, statements, 1)
	require.Contains(t, statements[0].Query,
		"COALESCE(SUM(SUM("+quoteIdent("amount")+")) OVER (PARTITION BY "+quoteIdent("category")+"), 0) AS "+quoteIdent("category_total"),
		"over groups the window aggregates the group measure, so the SUM nests")
}

func TestSelectWindowProjectsColumnsAsStored(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	// A row-level select projects plain columns as they are stored; a time
	// column reads back the UTC wall clock the framework writes on every
	// dialect, and a bucket is a plain expression next to them.
	type row struct {
		ID         string
		OccurredAt time.Time
		Day        string
		Rn         int64
	}
	rows := make([]row, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.ID, TestAggregateRecordCols.OccurredAt,
		TestAggregateRecordCols.OccurredAt.ByDay().As("day"),
		types.RowNumber().Over(types.PartitionBy(TestAggregateRecordCols.OccurredAt.ByDay()).OrderBy(TestAggregateRecordCols.ID.Asc())).As("rn"),
	).OrderBy(TestAggregateRecordCols.ID.Asc()).Limit(2).Scan(&rows))
	// The wall clock is compared in UTC: a driver may hand the instant back
	// in its own zone value, which is the same instant and a different Go
	// value.
	for i := range rows {
		rows[i].OccurredAt = rows[i].OccurredAt.UTC()
	}
	require.Equal(t, []row{
		{ID: "a1", OccurredAt: time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC), Day: "2024-01-10", Rn: 1},
		{ID: "a2", OccurredAt: time.Date(2024, 1, 10, 9, 0, 0, 0, time.UTC), Day: "2024-01-10", Rn: 2},
	}, rows)
}

func TestSelectWindowBuildErrors(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)
	ctx := context.Background()
	type row struct {
		ID string
		Rn int64
	}
	ordered := types.PartitionBy(TestAggregateRecordCols.Category).OrderBy(TestAggregateRecordCols.OccurredAt.Asc())
	scan := func(sel types.Selector[*TestAggregateRecord, row]) error {
		rows := make([]row, 0)
		return sel.Scan(&rows)
	}

	t.Run("WindowFunctionNeedsAWindow", func(t *testing.T) {
		require.ErrorIs(t, scan(database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID, types.RowNumber().As("rn"))),
			database.ErrWindowFnWithoutWindow)
	})

	t.Run("RankingNeedsAnOrderedWindow", func(t *testing.T) {
		require.ErrorIs(t, scan(database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID,
			types.RowNumber().Over(types.PartitionBy(TestAggregateRecordCols.Category)).As("rn"))),
			database.ErrWindowWithoutOrder)
	})

	t.Run("CountDistinctCannotBeWindowed", func(t *testing.T) {
		require.ErrorIs(t, scan(database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID,
			TestAggregateRecordCols.Category.CountDistinct().Over(ordered).As("rn"))),
			database.ErrWindowCountDistinct)
	})

	t.Run("KeyCannotBeWindowed", func(t *testing.T) {
		require.ErrorIs(t, scan(database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID,
			TestAggregateRecordCols.Category.Group().Over(ordered).As("rn"))),
			database.ErrWindowOnKey)
	})

	t.Run("PlainColumnNextToAnAggregate", func(t *testing.T) {
		require.ErrorIs(t, scan(database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID,
			TestAggregateRecordCols.Amount.Sum().As("rn"))),
			database.ErrPlainColumnInGroupedSelect)
	})

	t.Run("QualifyNeedsAWindowTerm", func(t *testing.T) {
		rn := types.RowNumber().Over(ordered).As("rn")
		require.ErrorIs(t, scan(database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID, rn).
			Qualify(TestAggregateRecordCols.Amount.Sum().As("rn").Gt(1))),
			database.ErrQualifyTermNotWindow)
	})

	t.Run("PartitionKeyCannotCarryConditions", func(t *testing.T) {
		// A key's conditions are never rendered, in a partition as in the
		// projection, so they are refused rather than dropped.
		require.ErrorIs(t, scan(database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID,
			types.RowNumber().Over(types.PartitionBy(TestAggregateRecordCols.Category.Group().Where(TestAggregateRecordCols.Status.Eq("done"))).OrderBy(TestAggregateRecordCols.ID.Asc())).As("rn"))),
			database.ErrConditionOnGroupKey)
	})

	t.Run("EitherWindowSpellingFindsTheTerm", func(t *testing.T) {
		// OrderBy is the short spelling of PartitionBy().OrderBy: a term
		// declared with one is the same term written with the other.
		type ranked struct {
			ID   string
			Rank int64
		}
		short := types.Rank().Over(types.OrderBy(TestAggregateRecordCols.Amount.Desc())).As("rank")
		long := types.Rank().Over(types.PartitionBy().OrderBy(TestAggregateRecordCols.Amount.Desc())).As("rank")
		rows := make([]ranked, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, ranked](ctx, TestAggregateRecordCols.ID, short).
			Qualify(long.Lte(2)).
			OrderBy(long.Asc()).
			Scan(&rows))
		require.Equal(t, []ranked{{ID: "a6", Rank: 1}, {ID: "a5", Rank: 2}}, rows)
	})

	t.Run("HavingCannotReadAWindowTerm", func(t *testing.T) {
		// A window is computed after HAVING; every dialect rejects the
		// statement, so the builder refuses it and points at Qualify.
		type grouped struct {
			Category string
			Total    int64
			Rn       int64
		}
		total := TestAggregateRecordCols.Amount.Sum().As("total")
		running := total.Over(types.OrderBy(total.Desc())).As("rn")
		err := database.Select[*TestAggregateRecord, grouped](ctx, TestAggregateRecordCols.Category.Group(), total, running).
			Having(running.Gt(100)).
			Scan(&[]grouped{})
		require.ErrorIs(t, err, database.ErrHavingWindowTerm)
	})

	t.Run("ConditionValueOfAnotherKind", func(t *testing.T) {
		// A row number is a number and a count is a number: text compared
		// against either is a comparison SQLite answers with no rows rather
		// than an error, so it is refused when the query is built.
		rn := types.RowNumber().Over(ordered).As("rn")
		require.ErrorIs(t, scan(database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID, rn).
			Qualify(rn.Eq("one"))),
			database.ErrHavingValueType)
		type counted struct {
			Category string
			Rn       int64
		}
		records := types.Count().As("rn")
		require.ErrorIs(t, database.Select[*TestAggregateRecord, counted](ctx, TestAggregateRecordCols.Category.Group(), records).
			Having(records.Gte("many")).
			Scan(&[]counted{}),
			database.ErrHavingValueType)
		// A number against a count, and an instant against the latest
		// occurrence, are the kinds the terms yield and pass.
		require.NoError(t, database.Select[*TestAggregateRecord, counted](ctx, TestAggregateRecordCols.Category.Group(), records).
			Having(records.Gte(2)).
			Scan(&[]counted{}))
		type latest struct {
			Category string
			Last     *time.Time
		}
		last := TestAggregateRecordCols.OccurredAt.Max().As("last")
		require.NoError(t, database.Select[*TestAggregateRecord, latest](ctx, TestAggregateRecordCols.Category.Group(), last).
			Having(last.Gt(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))).
			Scan(&[]latest{}))
	})

	t.Run("HavingNeedsGroups", func(t *testing.T) {
		rn := types.RowNumber().Over(ordered).As("rn")
		require.ErrorIs(t, scan(database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID, rn).
			Having(rn.Gt(1))),
			database.ErrHavingWithoutGroups)
	})

	t.Run("PartitionByUnknownColumn", func(t *testing.T) {
		require.ErrorIs(t, scan(database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID,
			types.RowNumber().Over(types.PartitionBy(colStatus).OrderBy(TestAggregateRecordCols.ID.Asc())).As("rn"))),
			database.ErrColumnTable, "a key of another model is refused by its table")
		missing := types.NewColumn[*TestAggregateRecord, string]("missing")
		require.ErrorIs(t, scan(database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID,
			types.RowNumber().Over(types.PartitionBy(missing).OrderBy(TestAggregateRecordCols.ID.Asc())).As("rn"))),
			database.ErrUnknownColumn)
	})

	t.Run("GroupedWindowPartitionsByAKey", func(t *testing.T) {
		type grouped struct {
			Category string
			Rn       int64
		}
		rows := make([]grouped, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, grouped](ctx,
			TestAggregateRecordCols.Category.Group(),
			types.Count().Over(types.PartitionBy(TestAggregateRecordCols.Status)).As("rn")).
			Scan(&rows), database.ErrWindowTermNotSelected)
	})

	t.Run("AverageCannotBeWindowedOverGroups", func(t *testing.T) {
		type grouped struct {
			Category string
			Rn       float64
		}
		rows := make([]grouped, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, grouped](ctx,
			TestAggregateRecordCols.Category.Group(),
			TestAggregateRecordCols.Amount.Avg().Over(types.PartitionBy(TestAggregateRecordCols.Category)).As("rn")).
			Scan(&rows), database.ErrWindowOverGroups)
	})

	t.Run("WindowCannotOrderByAWindow", func(t *testing.T) {
		inner := types.RowNumber().Over(ordered).As("inner")
		require.ErrorIs(t, scan(database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID,
			types.Rank().Over(types.OrderBy(inner.Asc())).As("rn"))),
			database.ErrWindowNested)
	})

	t.Run("ScanOneRejectsARowLevelSelect", func(t *testing.T) {
		got := row{}
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID,
			types.RowNumber().Over(ordered).As("rn")).ScanOne(&got),
			database.ErrScanOneRowLevel)
	})

	t.Run("OrderByAnUnselectedColumn", func(t *testing.T) {
		require.ErrorIs(t, scan(database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.ID,
			types.RowNumber().Over(ordered).As("rn")).OrderBy(TestAggregateRecordCols.Amount.Asc())),
			database.ErrOrderTermNotSelected)
	})

	t.Run("LagNeedsANullableField", func(t *testing.T) {
		type plain struct {
			ID       string
			Previous int64
		}
		rows := make([]plain, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, plain](ctx, TestAggregateRecordCols.ID,
			TestAggregateRecordCols.Amount.Lag().Over(ordered).As("previous")).Scan(&rows),
			database.ErrNullableResultField)
	})

	t.Run("NullableColumnNeedsANullableField", func(t *testing.T) {
		type plain struct {
			ID       string
			ClosedAt time.Time
			Rn       int64
		}
		rows := make([]plain, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, plain](ctx, TestAggregateRecordCols.ID,
			TestAggregateRecordCols.ClosedAt, types.RowNumber().Over(ordered).As("rn")).Scan(&rows),
			database.ErrNullableResultField)
	})
}
