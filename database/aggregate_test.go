package database_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

// The aggregate tests run against seeded rows rather than generated SQL: an
// aggregate that builds plausible SQL but returns a wrong number is exactly
// the failure these tests exist to catch, and only real rows show it.
//
// The seed is small and hand-checkable. Every expectation below is written as
// a literal, not computed from the fixture, so a wrong implementation cannot
// make the assertion agree with it.
//
//	id | category | status | amount | score | occurred_at
//	a1 | alpha    | done   |    100 |   1.5 | 2024-01-10 08:00
//	a2 | alpha    | done   |    200 |   2.5 | 2024-01-10 09:00
//	a3 | alpha    | failed |    300 |   3.5 | 2024-01-11 08:00
//	a4 | beta     | done   |    400 |   4.5 | 2024-02-10 08:00
//	a5 | beta     | failed |    500 |   5.5 | 2024-02-10 08:00
//	a6 | gamma    | done   |    600 |   6.5 | 2024-02-11 10:00
//
// Times are built in time.Local because the MySQL DSN carries loc=Local, so a
// stored timestamp round-trips to the same wall clock and the bucket labels
// are the literal strings below.

func aggregateSeed() []*TestAggregateRecord {
	at := func(month, day, hour int) time.Time {
		return time.Date(2024, time.Month(month), day, hour, 0, 0, 0, time.Local)
	}
	return []*TestAggregateRecord{
		{Base: model.Base{ID: "a1"}, Category: "alpha", Status: "done", Amount: 100, Score: 1.5, OccurredAt: at(1, 10, 8)},
		{Base: model.Base{ID: "a2"}, Category: "alpha", Status: "done", Amount: 200, Score: 2.5, OccurredAt: at(1, 10, 9)},
		{Base: model.Base{ID: "a3"}, Category: "alpha", Status: "failed", Amount: 300, Score: 3.5, OccurredAt: at(1, 11, 8)},
		{Base: model.Base{ID: "a4"}, Category: "beta", Status: "done", Amount: 400, Score: 4.5, OccurredAt: at(2, 10, 8)},
		{Base: model.Base{ID: "a5"}, Category: "beta", Status: "failed", Amount: 500, Score: 5.5, OccurredAt: at(2, 10, 8)},
		{Base: model.Base{ID: "a6"}, Category: "gamma", Status: "done", Amount: 600, Score: 6.5, OccurredAt: at(2, 11, 10)},
	}
}

// setupAggregateData clears the table and inserts the seed.
func setupAggregateData(t *testing.T) {
	t.Helper()
	cleanupAggregateData()
	require.NoError(t, database.Database[*TestAggregateRecord](context.Background()).Create(aggregateSeed()...))
}

// cleanupAggregateData removes every row, including the soft-deleted ones a
// List cannot see but which would still be counted by a later seed.
func cleanupAggregateData() {
	_ = database.DB().Exec("DELETE FROM test_aggregate_records").Error
}

// Column references for the fixture. gg gen writes these next to a real
// model; the database package has no generated code, so the tests build the
// same values by hand and exercise the same API.
var aggCols = struct {
	Category   types.Column[string]
	Status     types.Column[string]
	Amount     types.NumericColumn[int64]
	Score      types.NumericColumn[float64]
	OccurredAt types.TimeColumn
	ClosedAt   types.TimeColumn
}{
	Category:   types.NewColumn[string]("category"),
	Status:     types.NewColumn[string]("status"),
	Amount:     types.NewNumericColumn[int64]("amount"),
	Score:      types.NewNumericColumn[float64]("score"),
	OccurredAt: types.NewTimeColumn("occurred_at"),
	ClosedAt:   types.NewTimeColumn("closed_at"),
}

func TestAggregateScalar(t *testing.T) {
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
	require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
		Select(
			aggCols.Amount.Sum().As("total"),
			types.Count().As("records"),
			aggCols.Amount.Min().As("smallest"),
			aggCols.Amount.Max().As("largest"),
			aggCols.Score.Avg().As("avg_score"),
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

func TestAggregateGroupBy(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Category string
		Total    int64
		Records  int64
	}
	rows := make([]row, 0)
	require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
		Select(
			aggCols.Category.Group(),
			aggCols.Amount.Sum().As("total"),
			types.Count().As("records"),
		).
		OrderBy(aggCols.Category.Group().Asc()).
		Scan(&rows))

	require.Equal(t, []row{
		{Category: "alpha", Total: 600, Records: 3},
		{Category: "beta", Total: 900, Records: 2},
		{Category: "gamma", Total: 600, Records: 1},
	}, rows)
}

func TestAggregateConditional(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	// One scan produces a column per status. Without conditional aggregation
	// this report costs one full scan per status column.
	type row struct {
		Category    string
		DoneAmount  int64
		FailAmount  int64
		DoneRecords int64
	}
	rows := make([]row, 0)
	require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
		Select(
			aggCols.Category.Group(),
			aggCols.Amount.Sum().Where(aggCols.Status.Eq("done")).As("done_amount"),
			aggCols.Amount.Sum().Where(aggCols.Status.Eq("failed")).As("fail_amount"),
			types.Count().Where(aggCols.Status.Eq("done")).As("done_records"),
		).
		OrderBy(aggCols.Category.Group().Asc()).
		Scan(&rows))

	require.Equal(t, []row{
		{Category: "alpha", DoneAmount: 300, FailAmount: 300, DoneRecords: 2},
		{Category: "beta", DoneAmount: 400, FailAmount: 500, DoneRecords: 1},
		// gamma has no failed row: an empty SUM is coalesced to 0, never NULL.
		{Category: "gamma", DoneAmount: 600, FailAmount: 0, DoneRecords: 1},
	}, rows)
}

func TestAggregateHavingAndTopN(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Category string
		Total    int64
	}
	total := aggCols.Amount.Sum().As("total")

	t.Run("HavingFiltersGroups", func(t *testing.T) {
		rows := make([]row, 0)
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
			Select(aggCols.Category.Group(), total).
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
		done := types.Count().Where(aggCols.Status.Eq("done")).As("done")
		rows := make([]condRow, 0)
		require.NoError(t, database.Aggregate[*TestAggregateRecord, condRow](context.Background()).
			Select(aggCols.Category.Group(), done).
			Having(done.Gt(1)).
			Scan(&rows))
		// done rows per category: alpha 2, beta 1, gamma 1.
		require.Equal(t, []condRow{{Category: "alpha", Done: 2}}, rows)
	})

	t.Run("OrderByMeasureWithLimit", func(t *testing.T) {
		// alpha and gamma tie on 600, so the group key breaks the tie: without
		// it the databases would be free to answer either group second.
		rows := make([]row, 0)
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
			Select(aggCols.Category.Group(), total).
			OrderBy(total.Desc(), aggCols.Category.Group().Asc()).
			Limit(2).
			Scan(&rows))
		require.Equal(t, []row{{Category: "beta", Total: 900}, {Category: "alpha", Total: 600}}, rows)
	})

	t.Run("OffsetPagesGroups", func(t *testing.T) {
		rows := make([]row, 0)
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
			Select(aggCols.Category.Group(), total).
			OrderBy(total.Desc(), aggCols.Category.Group().Asc()).
			Limit(1).
			Offset(1).
			Scan(&rows))
		require.Equal(t, []row{{Category: "alpha", Total: 600}}, rows)
	})
}

func TestAggregateCountDistinct(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Categories int64
	}
	got := row{}
	require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
		Select(aggCols.Category.CountDistinct().As("categories")).
		ScanOne(&got))
	require.EqualValues(t, 3, got.Categories)
}

func TestAggregateTimeBucket(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Bucket  string
		Total   int64
		Records int64
	}

	t.Run("ByDay", func(t *testing.T) {
		rows := make([]row, 0)
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
			Select(
				aggCols.OccurredAt.ByDay().As("bucket"),
				aggCols.Amount.Sum().As("total"),
				types.Count().As("records"),
			).
			OrderBy(aggCols.OccurredAt.ByDay().As("bucket").Asc()).
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
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
			Select(
				aggCols.OccurredAt.ByMonth().As("bucket"),
				aggCols.Amount.Sum().As("total"),
				types.Count().As("records"),
			).
			OrderBy(aggCols.OccurredAt.ByMonth().As("bucket").Asc()).
			Scan(&rows))

		require.Equal(t, []row{
			{Bucket: "2024-01", Total: 600, Records: 3},
			{Bucket: "2024-02", Total: 1500, Records: 3},
		}, rows)
	})

	t.Run("ByHour", func(t *testing.T) {
		skipOnDialect(t, config.DBSqlite, "BUG-6: sqlite stores timestamps with a zone offset and strftime converts them to UTC, so bucket labels disagree with the local-time labels MySQL renders")
		skipOnDialect(t, config.DBPostgres, "BUG-6: postgres renders timestamptz in the session time zone (UTC by framework default), so bucket labels disagree with the local-time labels MySQL renders")
		rows := make([]row, 0)
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
			Select(
				aggCols.OccurredAt.ByHour().As("bucket"),
				aggCols.Amount.Sum().As("total"),
				types.Count().As("records"),
			).
			Where(aggCols.Category.Eq("alpha")).
			OrderBy(aggCols.OccurredAt.ByHour().As("bucket").Asc()).
			Scan(&rows))

		require.Equal(t, []row{
			{Bucket: "2024-01-10 08:00:00", Total: 100, Records: 1},
			{Bucket: "2024-01-10 09:00:00", Total: 200, Records: 1},
			{Bucket: "2024-01-11 08:00:00", Total: 300, Records: 1},
		}, rows)
	})
}

func TestAggregateWhereReusesFilters(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Total int64
	}
	got := row{}

	// A filter group is AND-combined with the rest, so the category condition
	// cannot be absorbed into the OR.
	require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
		Select(aggCols.Amount.Sum().As("total")).
		Where(
			aggCols.Category.Eq("alpha"),
			types.FilterOr(aggCols.Status.Eq("failed"), aggCols.Amount.Gte(200)),
		).
		ScanOne(&got))
	// alpha rows: 100/done excluded, 200/done kept by amount, 300/failed kept.
	require.EqualValues(t, 500, got.Total)
}

func TestAggregateCountGroups(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Category string
		Total    int64
	}
	total := aggCols.Amount.Sum().As("total")

	t.Run("CountsGroupsNotRows", func(t *testing.T) {
		var groups int
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
			Select(aggCols.Category.Group(), total).
			CountGroups(&groups))
		require.Equal(t, 3, groups, "six rows fall into three categories")
	})

	t.Run("RespectsHaving", func(t *testing.T) {
		var groups int
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
			Select(aggCols.Category.Group(), total).
			Having(total.Gt(600)).
			CountGroups(&groups))
		require.Equal(t, 1, groups)
	})

	t.Run("InnerProjectsOnlyGroupKeys", func(t *testing.T) {
		// The outer count reads nothing but how many rows the derived table
		// answers, so a measure inside it would be computed for every group
		// and then thrown away. HAVING keeps working without the measure in
		// the select list because it renders its own expression.
		var groups int
		statements := make([]types.SQLStatement, 0)
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
			WithBuildSQL(&statements).
			Select(aggCols.Category.Group(), total).
			Having(total.Gt(600)).
			CountGroups(&groups))
		require.Len(t, statements, 1)
		sql := statements[0].RenderedSQL
		require.Contains(t, sql, "GROUP BY")
		projection, having, hasHaving := strings.Cut(sql, "HAVING")
		require.True(t, hasHaving, "the count must keep filtering groups")
		require.Contains(t, having, "SUM(", "HAVING renders the full measure expression")
		require.NotContains(t, projection, "SUM(", "the count projection must not compute measures")
	})

	t.Run("CountsOneGroupWithoutKeys", func(t *testing.T) {
		// Without group keys the whole read is a single group, and the count
		// answers one even though nothing scans the measure values.
		var groups int
		require.NoError(t, database.Aggregate[*TestAggregateRecord, struct{ Total int64 }](context.Background()).
			Select(total).
			CountGroups(&groups))
		require.Equal(t, 1, groups)
	})
}

// TestAggregateHidesSoftDeletedRows is the regression test for the failure an
// aggregate is most likely to have: it scans into a plain result row, so gorm
// parses no model and the soft-delete condition silently disappears unless the
// model is attached to the statement.
func TestAggregateHidesSoftDeletedRows(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	ctx := context.Background()
	gamma := &TestAggregateRecord{Base: model.Base{ID: "a6"}}
	require.NoError(t, database.Database[*TestAggregateRecord](ctx).Delete(gamma))

	// The row is soft deleted, so List no longer sees it.
	remaining := make([]*TestAggregateRecord, 0)
	require.NoError(t, database.Database[*TestAggregateRecord](ctx).List(&remaining))
	require.Len(t, remaining, 5)

	type row struct {
		Total   int64
		Records int64
	}
	got := row{}
	require.NoError(t, database.Aggregate[*TestAggregateRecord, row](ctx).
		Select(aggCols.Amount.Sum().As("total"), types.Count().As("records")).
		ScanOne(&got))

	require.EqualValues(t, 5, got.Records, "aggregate must not count soft-deleted rows")
	require.EqualValues(t, 1500, got.Total, "2100 minus the soft-deleted 600")

	type groupRow struct {
		Category string
		Total    int64
	}
	var groups int
	require.NoError(t, database.Aggregate[*TestAggregateRecord, groupRow](ctx).
		Select(aggCols.Category.Group(), aggCols.Amount.Sum().As("total")).
		CountGroups(&groups))
	require.Equal(t, 2, groups, "the gamma group disappears with its only row")
}

func TestAggregateBuildErrors(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	ctx := context.Background()
	type row struct {
		Category string
		Total    int64
	}

	t.Run("EmptyProjection", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).Scan(&rows), database.ErrEmptyProjection)
	})

	t.Run("ProjectionWithoutAggregateFunction", func(t *testing.T) {
		// Group keys alone are a plain read, which List already does.
		type keyOnly struct{ Category string }
		rows := make([]keyOnly, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, keyOnly](ctx).
			Select(aggCols.Category.Group()).
			Scan(&rows), database.ErrNoAggregateFn)
	})

	t.Run("UnknownColumn", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Category.Group(), types.SumOf("nonexistent").As("total")).
			Scan(&rows), database.ErrUnknownColumn)
	})

	t.Run("SumOverNonNumericColumnViaStringPath", func(t *testing.T) {
		// The typed path cannot express this: Column[string] has no Sum. The
		// string constructor can, so the build-time rule table has to stop it,
		// otherwise MySQL answers with 0 and a warning.
		rows := make([]row, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Category.Group(), types.SumOf("status").As("total")).
			Scan(&rows), database.ErrAggregateType)
	})

	t.Run("TimeBucketOverNonTimeColumn", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(types.ByDayOf("amount").As("category"), aggCols.Amount.Sum().As("total")).
			Scan(&rows), database.ErrAggregateType)
	})

	t.Run("DuplicateAlias", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Amount.Sum().As("total"), aggCols.Amount.Max().As("total")).
			Scan(&rows), database.ErrDuplicateAlias)
	})

	t.Run("ResultRowMissingFieldForAlias", func(t *testing.T) {
		// gorm would leave the field zero and drop the column silently, which
		// reaches a report as a column of zeros.
		type missing struct{ Category string }
		rows := make([]missing, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, missing](ctx).
			Select(aggCols.Category.Group(), aggCols.Amount.Sum().As("total")).
			Scan(&rows), database.ErrResultFieldMissing)
	})

	t.Run("ProjectionMissingAliasForResultField", func(t *testing.T) {
		type extra struct {
			Category string
			Total    int64
			Unbound  int64
		}
		rows := make([]extra, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, extra](ctx).
			Select(aggCols.Category.Group(), aggCols.Amount.Sum().As("total")).
			Scan(&rows), database.ErrAliasMissing)
	})

	t.Run("HavingReferencesUnselectedMeasure", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Category.Group(), aggCols.Amount.Sum().As("total")).
			Having(aggCols.Score.Avg().As("avg_score").Gt(1)).
			Scan(&rows), database.ErrHavingTermNotSelected)
	})

	t.Run("UnknownAggregateFunction", func(t *testing.T) {
		// The renderer composes SQL from the constant, so a value from outside
		// the closed set would otherwise reach the statement as text.
		rows := make([]row, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Category.Group(), types.AggregateTerm{
				Fn: "TOTALLY_NOT_SQL", Column: "amount", Alias: "total",
			}).
			Scan(&rows), database.ErrUnknownAggregateFn)
	})

	t.Run("UnknownTimeBucket", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(types.AggregateTerm{Column: "occurred_at", Bucket: "fortnight", Alias: "category"},
				aggCols.Amount.Sum().As("total")).
			Scan(&rows), database.ErrUnknownTimeBucket)
	})

	t.Run("ConditionOnGroupKey", func(t *testing.T) {
		// Conditions only restrict a measure. They used to be dropped without a
		// word, which reads as a report quietly counting the wrong rows.
		rows := make([]row, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Category.Group().Where(aggCols.Status.Eq("done")),
				aggCols.Amount.Sum().As("total")).
			Scan(&rows), database.ErrConditionOnGroupKey)
	})

	t.Run("BucketOnMeasure", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Category.Group(), types.AggregateTerm{
				Fn: types.AggregateSum, Column: "amount", Bucket: types.TimeBucketDay, Alias: "total",
			}).
			Scan(&rows), database.ErrBucketOnMeasure)
	})

	t.Run("HavingTermDiffersFromProjectedTerm", func(t *testing.T) {
		// Same alias, different expression: HAVING renders its own term, so
		// matching the alias alone would filter by a measure the projection
		// never declared.
		type condRow struct {
			Category string
			Done     int64
		}
		rows := make([]condRow, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, condRow](ctx).
			Select(aggCols.Category.Group(), types.Count().Where(aggCols.Status.Eq("done")).As("done")).
			Having(types.Count().As("done").Gt(1)).
			Scan(&rows), database.ErrHavingTermNotSelected)
	})

	t.Run("ConditionalMeasureNeedsPointerField", func(t *testing.T) {
		// A condition can pass no row of a group, and MAX over nothing is
		// NULL even though the group itself is not empty.
		type flat struct {
			Category string
			Peak     int64
		}
		rows := make([]flat, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, flat](ctx).
			Select(aggCols.Category.Group(),
				aggCols.Amount.Max().Where(aggCols.Status.Eq("done")).As("peak")).
			Scan(&rows), database.ErrNullableResultField)
	})

	t.Run("UnusableFilterFailsFastInsteadOfEmptyReport", func(t *testing.T) {
		// A client filter that cannot be applied narrows the query. Here the
		// same predicate would turn a report into a silent zero, so it errors.
		got := struct{ Total int64 }{}
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, struct{ Total int64 }](ctx).
			Select(aggCols.Amount.Sum().As("total")).
			Where(types.FilterOr()).
			ScanOne(&got), database.ErrUnusableFilter)
	})

	t.Run("OffsetWithoutLimit", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Category.Group(), aggCols.Amount.Sum().As("total")).
			Offset(1).
			Scan(&rows), database.ErrOffsetWithoutLimit)
	})

	t.Run("UnknownHavingOperator", func(t *testing.T) {
		rows := make([]row, 0)
		total := aggCols.Amount.Sum().As("total")
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Category.Group(), total).
			Having(types.Having{Term: total, Op: "approximately", Value: 1}).
			Scan(&rows), database.ErrUnknownHavingOp)
	})

	t.Run("UnknownOrderDirection", func(t *testing.T) {
		rows := make([]row, 0)
		total := aggCols.Amount.Sum().As("total")
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Category.Group(), total).
			OrderBy(types.AggregateOrder{Term: total, Direction: "sideways"}).
			Scan(&rows), database.ErrUnknownOrderDirection)
	})

	t.Run("OrderByTermDiffersFromProjectedTerm", func(t *testing.T) {
		// Same alias, different expression: ORDER BY renders its own term, so
		// matching the alias alone would sort by a measure the projection never
		// declared.
		type condRow struct {
			Category string
			Done     int64
		}
		rows := make([]condRow, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, condRow](ctx).
			Select(aggCols.Category.Group(), types.Count().Where(aggCols.Status.Eq("done")).As("done")).
			OrderBy(types.Count().As("done").Desc()).
			Scan(&rows), database.ErrOrderTermNotSelected)
	})

	// ScanOne rejects all three pagination inputs, not just the one that
	// happened to be covered.
	for name, build := range map[string]func(types.Aggregator[*TestAggregateRecord, struct{ Total int64 }]) types.Aggregator[*TestAggregateRecord, struct{ Total int64 }]{
		"Limit": func(a types.Aggregator[*TestAggregateRecord, struct{ Total int64 }]) types.Aggregator[*TestAggregateRecord, struct{ Total int64 }] {
			return a.Limit(1)
		},
		"Offset": func(a types.Aggregator[*TestAggregateRecord, struct{ Total int64 }]) types.Aggregator[*TestAggregateRecord, struct{ Total int64 }] {
			return a.Limit(1).Offset(1)
		},
		"Having": func(a types.Aggregator[*TestAggregateRecord, struct{ Total int64 }]) types.Aggregator[*TestAggregateRecord, struct{ Total int64 }] {
			return a.Having(aggCols.Amount.Sum().As("total").Gt(1))
		},
	} {
		t.Run("ScanOneRejects"+name, func(t *testing.T) {
			got := struct{ Total int64 }{}
			base := database.Aggregate[*TestAggregateRecord, struct{ Total int64 }](ctx).
				Select(aggCols.Amount.Sum().As("total"))
			require.ErrorIs(t, build(base).ScanOne(&got), database.ErrScanOnePaged)
		})
	}

	t.Run("ScanOneRejectsGroupedQuery", func(t *testing.T) {
		got := row{}
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Category.Group(), aggCols.Amount.Sum().As("total")).
			ScanOne(&got), database.ErrGroupedScanOne)
	})
}

func TestAggregateBuildSQL(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Category string
		Total    int64
	}
	rows := make([]row, 0)
	statements := make([]types.SQLStatement, 0)
	require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
		WithBuildSQL(&statements).
		Select(aggCols.Category.Group(), aggCols.Amount.Sum().As("total")).
		Scan(&rows))

	require.Len(t, statements, 1)
	sql := statements[0].RenderedSQL
	require.Contains(t, sql, "COALESCE(SUM(")
	require.Contains(t, sql, "GROUP BY")
	require.Contains(t, sql, "deleted_at")
	require.Empty(t, rows, "dry run builds SQL without reading rows")
}

// tagCols mirrors the generated column references of the related model.
var tagCols = struct {
	ID       types.Column[string]
	RecordID types.Column[string]
	Label    types.Column[string]
}{
	ID:       types.NewColumn[string]("id"),
	RecordID: types.NewColumn[string]("record_id"),
	Label:    types.NewColumn[string]("label"),
}

var recordIDCol = types.NewColumn[string]("id")

// setupTagData seeds tags on a1, a3 and a4. a1 and a3 are alpha rows, a4 is a
// beta row, so a subquery on the "vip" label selects across categories.
func setupTagData(t *testing.T) {
	t.Helper()
	cleanupTagData()
	require.NoError(t, database.Database[*TestRecordTag](context.Background()).Create(
		&TestRecordTag{Base: model.Base{ID: "t1"}, RecordID: "a1", Label: "vip"},
		&TestRecordTag{Base: model.Base{ID: "t2"}, RecordID: "a3", Label: "vip"},
		&TestRecordTag{Base: model.Base{ID: "t3"}, RecordID: "a4", Label: "bulk"},
	))
}

func cleanupTagData() {
	_ = database.DB().Exec("DELETE FROM test_record_tags").Error
}

func TestFilterExists(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	vip := types.FilterExists[*TestRecordTag](tagCols.RecordID, recordIDCol, tagCols.Label.Eq("vip"))

	// The same filter serves List and Aggregate: it is a Filter operator, not
	// an aggregate feature.
	t.Run("NarrowsList", func(t *testing.T) {
		records := make([]*TestAggregateRecord, 0)
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{vip}}).
			WithOrder(types.Asc("id")).
			List(&records))
		require.Len(t, records, 2)
		require.Equal(t, "a1", records[0].ID)
		require.Equal(t, "a3", records[1].ID)
	})

	t.Run("NarrowsAggregate", func(t *testing.T) {
		type row struct {
			Total   int64
			Records int64
		}
		got := row{}
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Amount.Sum().As("total"), types.Count().As("records")).
			Where(vip).
			ScanOne(&got))
		require.EqualValues(t, 2, got.Records)
		require.EqualValues(t, 400, got.Total, "a1 is 100 and a3 is 300")
	})

	t.Run("CountsEachRowOnce", func(t *testing.T) {
		// a1 gets a second vip tag. A join would duplicate the row and double
		// its amount; a semi join matches it once.
		require.NoError(t, database.Database[*TestRecordTag](ctx).Create(
			&TestRecordTag{Base: model.Base{ID: "t4"}, RecordID: "a1", Label: "vip"},
		))
		type row struct {
			Total   int64
			Records int64
		}
		got := row{}
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Amount.Sum().As("total"), types.Count().As("records")).
			Where(vip).
			ScanOne(&got))
		require.EqualValues(t, 2, got.Records, "a second tag must not duplicate the row")
		require.EqualValues(t, 400, got.Total)
	})

	t.Run("HidesSoftDeletedRelatedRows", func(t *testing.T) {
		require.NoError(t, database.Database[*TestRecordTag](ctx).Delete(
			&TestRecordTag{Base: model.Base{ID: "t2"}},
		))
		records := make([]*TestAggregateRecord, 0)
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{vip}}).
			List(&records))
		require.Len(t, records, 1, "a3 loses its only live vip tag")
		require.Equal(t, "a1", records[0].ID)
	})
}

func TestFilterNotExists(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	// Rows with no vip tag at all, plus rows whose tags are not vip.
	noVip := types.FilterNotExists[*TestRecordTag](tagCols.RecordID, recordIDCol, tagCols.Label.Eq("vip"))

	records := make([]*TestAggregateRecord, 0)
	require.NoError(t, database.Database[*TestAggregateRecord](ctx).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{noVip}}).
		WithOrder(types.Asc("id")).
		List(&records))

	ids := make([]string, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.ID)
	}
	require.Equal(t, []string{"a2", "a4", "a5", "a6"}, ids,
		"a4 carries only a bulk tag, so it counts as having no vip tag")
}

func TestFilterExistsCombinesWithOtherFilters(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	vip := types.FilterExists[*TestRecordTag](tagCols.RecordID, recordIDCol, tagCols.Label.Eq("vip"))

	records := make([]*TestAggregateRecord, 0)
	require.NoError(t, database.Database[*TestAggregateRecord](ctx).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{
			vip,
			aggCols.Status.Eq("failed"),
		}}).
		List(&records))
	require.Len(t, records, 1, "a1 is done and a3 is failed")
	require.Equal(t, "a3", records[0].ID)
}

// TestAggregateConditionalOnSubquery covers the combination a report reaches
// for when the measure's own table carries no flag to split on: the split
// lives in a related table, so the CASE predicate is a correlated subquery
// rather than a column comparison.
//
// A join would be the obvious alternative and the wrong one: joining a
// one-to-many child multiplies the outer rows, and the SUM then counts a row
// once per related row instead of once.
func TestAggregateConditionalOnSubquery(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	tagged := types.FilterExists[*TestRecordTag](tagCols.RecordID, recordIDCol, tagCols.Label.Eq("vip"))
	untagged := types.FilterNotExists[*TestRecordTag](tagCols.RecordID, recordIDCol, tagCols.Label.Eq("vip"))

	type row struct {
		TaggedAmount   int64
		UntaggedAmount int64
		TaggedRecords  int64
	}
	got := row{}
	require.NoError(t, database.Aggregate[*TestAggregateRecord, row](ctx).
		Select(
			aggCols.Amount.Sum().Where(tagged).As("tagged_amount"),
			aggCols.Amount.Sum().Where(untagged).As("untagged_amount"),
			types.Count().Where(tagged).As("tagged_records"),
		).
		ScanOne(&got))

	// vip tags sit on a1 (100) and a3 (300); the rest carry no vip tag.
	require.EqualValues(t, 400, got.TaggedAmount)
	require.EqualValues(t, 1700, got.UntaggedAmount)
	require.EqualValues(t, 2, got.TaggedRecords)
	require.EqualValues(t, 2100, got.TaggedAmount+got.UntaggedAmount,
		"the two subsets must partition the table exactly once")

	t.Run("SecondRelatedRowDoesNotDoubleCount", func(t *testing.T) {
		require.NoError(t, database.Database[*TestRecordTag](ctx).Create(
			&TestRecordTag{Base: model.Base{ID: "t9"}, RecordID: "a1", Label: "vip"},
		))
		again := row{}
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(
				aggCols.Amount.Sum().Where(tagged).As("tagged_amount"),
				aggCols.Amount.Sum().Where(untagged).As("untagged_amount"),
				types.Count().Where(tagged).As("tagged_records"),
			).
			ScanOne(&again))
		require.EqualValues(t, 400, again.TaggedAmount, "a semi join matches a row once")
		require.EqualValues(t, 2, again.TaggedRecords)
	})
}

// TestFilterExistsTableResolution pins the subquery's FROM to the same table
// its correlation qualifies. gorm names the FROM from the struct unless told
// otherwise, and it reads its own TableName method rather than the framework's
// GetTableName, so a model that overrides only the framework method would be
// selected FROM one table while the correlation referenced another.
func TestFilterExistsTableResolution(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	// TestTagAlias resolves to test_record_tags through GetTableName, while its
	// struct name would make gorm derive test_tag_aliases.
	vip := types.FilterExists[*TestTagAlias](tagCols.RecordID, recordIDCol, tagCols.Label.Eq("vip"))

	records := make([]*TestAggregateRecord, 0)
	require.NoError(t, database.Database[*TestAggregateRecord](ctx).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{vip}}).
		WithOrder(types.Asc("id")).
		List(&records))
	require.Len(t, records, 2)
	require.Equal(t, "a1", records[0].ID)
	require.Equal(t, "a3", records[1].ID)
}

// TestFilterExistsNested pins the inner correlation to the table directly
// enclosing it. Reading the outer chain instead would correlate the grandchild
// against the outermost model, which is valid SQL joined on the wrong table:
// it returns a wrong row set rather than an error.
func TestFilterExistsNested(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	defer func() { _ = database.DB().Exec("DELETE FROM test_tag_notes").Error }()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	// A note on t1 only. t1 tags a1, so a1 is the single record reachable
	// through tag -> note.
	require.NoError(t, database.Database[*TestTagNote](ctx).Create(
		&TestTagNote{Base: model.Base{ID: "n1"}, TagID: "t1", Body: "checked"},
	))

	noteCols := struct {
		TagID types.Column[string]
		Body  types.Column[string]
	}{
		TagID: types.NewColumn[string]("tag_id"),
		Body:  types.NewColumn[string]("body"),
	}
	tagIDCol := types.NewColumn[string]("id")

	// EXISTS(tag WHERE tag.record_id = record.id AND EXISTS(note WHERE
	// note.tag_id = tag.id AND note.body = 'checked'))
	//
	// The inner correlation must name the tag table. Naming the record table
	// would compare note.tag_id against record.id, which happens to be a legal
	// comparison of two id columns and silently matches nothing here.
	hasCheckedNote := types.FilterExists[*TestRecordTag](
		tagCols.RecordID, recordIDCol,
		types.FilterExists[*TestTagNote](noteCols.TagID, tagIDCol, noteCols.Body.Eq("checked")),
	)

	records := make([]*TestAggregateRecord, 0)
	require.NoError(t, database.Database[*TestAggregateRecord](ctx).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{hasCheckedNote}}).
		List(&records))
	require.Len(t, records, 1, "only a1 has a tag carrying a checked note")
	require.Equal(t, "a1", records[0].ID)
}

// TestFilterExistsFailsClosedUnderNegation pins the one place in the renderer
// where fail-closed could invert. A predicate that cannot be applied becomes
// "match nothing"; placing that inside NOT EXISTS would turn it into "match
// everything", which is the only way this package could widen a result set.
// The whole condition has to collapse instead of the inner one.
func TestFilterExistsFailsClosedUnderNegation(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	list := func(t *testing.T, f types.Filter) int {
		t.Helper()
		records := make([]*TestAggregateRecord, 0)
		require.NoError(t, database.Database[*TestAggregateRecord](ctx).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{f}}).
			List(&records))
		return len(records)
	}

	// An empty group cannot be rendered, so the subquery's filter fails closed.
	broken := types.FilterOr()

	t.Run("ExistsMatchesNothing", func(t *testing.T) {
		require.Equal(t, 0, list(t, types.FilterExists[*TestRecordTag](
			tagCols.RecordID, recordIDCol, broken,
		)))
	})

	t.Run("NotExistsAlsoMatchesNothing", func(t *testing.T) {
		require.Equal(t, 0, list(t, types.FilterNotExists[*TestRecordTag](
			tagCols.RecordID, recordIDCol, broken,
		)),
			"negating a fail-closed subquery must not return the whole table")
	})
}

// TestFilterExistsValidatesInnerColumns pins the subquery's filters to the
// related model's own columns. A name only the outer table has would otherwise
// resolve against the enclosing query and silently turn the condition into a
// correlated reference over different rows.
func TestFilterExistsValidatesInnerColumns(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	ctx := context.Background()
	// "category" exists on the record table but not on the tag table.
	outerOnly := types.FilterExists[*TestRecordTag](
		tagCols.RecordID, recordIDCol, aggCols.Category.Eq("alpha"),
	)

	records := make([]*TestAggregateRecord, 0)
	require.NoError(t, database.Database[*TestAggregateRecord](ctx).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{outerOnly}}).
		List(&records))
	require.Empty(t, records, "an unknown inner column fails closed instead of correlating outward")

	// The aggregate path reports the reason rather than answering with zero.
	got := struct{ Total int64 }{}
	require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, struct{ Total int64 }](ctx).
		Select(aggCols.Amount.Sum().As("total")).
		Where(outerOnly).
		ScanOne(&got), database.ErrUnusableFilter)
}

// TestFilterExistsQualifiesInnerColumns pins the shape of the generated SQL
// rather than its result. SQL resolves an unqualified name against the
// innermost scope first, so a subquery filter reaches the right column either
// way and no query result can tell the two spellings apart. The qualification
// is still worth pinning: it is what keeps the emitted condition unambiguous
// on its face, and what would keep it correct if a subquery ever gained a join.
func TestFilterExistsQualifiesInnerColumns(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)

	// Both tables carry an "id"; the filter names the tag's own.
	byTagID := types.FilterExists[*TestRecordTag](
		tagCols.RecordID, recordIDCol, tagCols.ID.Eq("t1"),
	)

	statements := make([]types.SQLStatement, 0)
	records := make([]*TestAggregateRecord, 0)
	require.NoError(t, database.Database[*TestAggregateRecord](context.Background()).
		WithBuildSQL(&statements).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{byTagID}}).
		List(&records))

	require.Len(t, statements, 1)
	require.Contains(t, statements[0].Query, quoteIdent("test_record_tags")+"."+quoteIdent("id")+" =",
		"the inner filter must name the subquery's own table")
}

// TestFilterExistsSelfJoin covers a related model that reads the same table as
// the query around it. Without a distinct name for the subquery both sides of
// the correlation resolve to the inner table and the condition degenerates
// into comparing a row with itself.
func TestFilterExistsSelfJoin(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	ctx := context.Background()
	catCols := struct {
		ID       types.Column[string]
		ParentID types.Column[string]
	}{
		ID:       types.NewColumn[string]("id"),
		ParentID: types.NewColumn[string]("parent_id"),
	}
	require.NoError(t, database.Database[*TestCategory](ctx).Create(categoryRoot, categoryParent))

	// Categories that are somebody's parent. root parents itself and parent,
	// parent has no children, so only root matches.
	hasChild := types.FilterExists[*TestCategory](catCols.ParentID, catCols.ID)
	cats := make([]*TestCategory, 0)
	require.NoError(t, database.Database[*TestCategory](ctx).
		WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: []types.Filter{hasChild}}).
		List(&cats))
	require.Len(t, cats, 1)
	require.Equal(t, categoryRootID, cats[0].ID)
}

// TestSumOfRejectsTextBackedValuer pins SUM to the single classification rule.
// Admitting any struct that stores itself through driver.Valuer would take in
// gorm.DeletedAt, which every model carries, and the null wrappers -- the exact
// types ClassifyColumn exists to keep out.
func TestSumOfRejectsTextBackedValuer(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	rows := make([]struct{ Total int64 }, 0)
	require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, struct{ Total int64 }](context.Background()).
		Select(types.SumOf("deleted_at").As("total")).
		Scan(&rows), database.ErrAggregateType)
}

// TestAggregateBuilderReuse pins the paginated-report idiom: read the page,
// then count the groups off the same builder. The chain's gorm session keeps
// the clauses of whatever ran on it before, so without a fresh statement per
// build the count inherits the page's LIMIT and reports the page size as the
// total -- silently, and only when tracing is off.
func TestAggregateBuilderReuse(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Category string
		Total    int64
	}
	ctx := context.Background()
	builder := database.Aggregate[*TestAggregateRecord, row](ctx).
		Select(aggCols.Category.Group(), aggCols.Amount.Sum().As("total")).
		Where(aggCols.Status.Eq("done")).
		OrderBy(aggCols.Category.Group().Asc()).
		Limit(1)

	rows := make([]row, 0)
	require.NoError(t, builder.Scan(&rows))
	require.Equal(t, []row{{Category: "alpha", Total: 300}}, rows)

	var groups int
	require.NoError(t, builder.CountGroups(&groups))
	require.Equal(t, 3, groups, "the page limit must not leak into the total")

	// A second identical read repeats the query rather than compounding it.
	again := make([]row, 0)
	require.NoError(t, builder.Scan(&again))
	require.Equal(t, rows, again)
}

// TestAggregateNullableResultFields pins where a result field must be able to
// tell NULL apart from zero. AVG, MIN and MAX return NULL when they see no
// value, so the field needs a pointer or a sql.Null wrapper — except for a
// grouped, unconditional measure over a non-nullable column, where every group
// holds at least one real value and a plain field cannot receive NULL.
func TestAggregateNullableResultFields(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	ctx := context.Background()

	t.Run("AcceptsSQLNullWrappers", func(t *testing.T) {
		type row struct {
			Peak     sql.NullInt64
			AvgScore sql.NullFloat64
		}
		got := row{}
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Amount.Max().As("peak"), aggCols.Score.Avg().As("avg_score")).
			ScanOne(&got))
		require.True(t, got.Peak.Valid)
		require.EqualValues(t, 600, got.Peak.Int64)
		require.True(t, got.AvgScore.Valid)
	})

	t.Run("UngroupedRejectsPlainField", func(t *testing.T) {
		// Without group keys the whole read is one group, and it is empty when
		// the filters match no rows.
		type row struct{ Peak int64 }
		got := row{}
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Amount.Max().As("peak")).
			ScanOne(&got), database.ErrNullableResultField)
	})

	t.Run("GroupedOverNonNullColumnAcceptsPlainField", func(t *testing.T) {
		// GROUP BY emits no empty groups and the source columns cannot store
		// NULL, so these measures always produce a value.
		type row struct {
			Category string
			Peak     int64
			Earliest int64
			AvgScore float64
		}
		rows := make([]row, 0)
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Category.Group(),
				aggCols.Amount.Max().As("peak"),
				aggCols.Amount.Min().As("earliest"),
				aggCols.Score.Avg().As("avg_score")).
			OrderBy(aggCols.Category.Group().Asc()).
			Scan(&rows))
		require.Equal(t, []row{
			{Category: "alpha", Peak: 300, Earliest: 100, AvgScore: 2.5},
			{Category: "beta", Peak: 500, Earliest: 400, AvgScore: 5.0},
			{Category: "gamma", Peak: 600, Earliest: 600, AvgScore: 6.5},
		}, rows)
	})

	t.Run("GroupedOverNullableColumnRejectsPlainField", func(t *testing.T) {
		// A group can hold rows whose closed_at is NULL throughout, and MAX
		// over only NULLs is NULL.
		type row struct {
			Category string
			LastSeen time.Time
		}
		rows := make([]row, 0)
		require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
			Select(aggCols.Category.Group(), aggCols.ClosedAt.Max().As("last_seen")).
			Scan(&rows), database.ErrNullableResultField)
	})
}

// TestAggregateHavingValue pins the values a post-aggregation comparison
// accepts. nil renders as a comparison against NULL, which no group satisfies,
// so a report would come back empty with no sign of the mistake.
func TestAggregateHavingValue(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	ctx := context.Background()
	type row struct {
		Category string
		Total    int64
	}
	total := aggCols.Amount.Sum().As("total")

	for name, value := range map[string]any{
		"Nil":              nil,
		"Slice":            []int64{1, 2},
		"TypedNilPointer":  (*int64)(nil),
		"NestedNilPointer": new(*int64),
	} {
		t.Run(name, func(t *testing.T) {
			rows := make([]row, 0)
			require.ErrorIs(t, database.Aggregate[*TestAggregateRecord, row](ctx).
				Select(aggCols.Category.Group(), total).
				Having(total.Gt(value)).
				Scan(&rows), database.ErrHavingValue)
		})
	}
}

// TestAggregateScanReplacesDest pins that a read replaces the destination
// rather than appending to it. gorm keeps the existing elements when a scan
// returns no rows, so a reused destination would still hold the previous
// result and the caller would read a stale report as a fresh one.
func TestAggregateScanReplacesDest(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	ctx := context.Background()
	type row struct {
		Category string
		Total    int64
	}
	rows := make([]row, 0)
	require.NoError(t, database.Aggregate[*TestAggregateRecord, row](ctx).
		Select(aggCols.Category.Group(), aggCols.Amount.Sum().As("total")).
		Scan(&rows))
	require.Len(t, rows, 3)

	// A second read matching nothing must empty it, not leave the first result.
	require.NoError(t, database.Aggregate[*TestAggregateRecord, row](ctx).
		Select(aggCols.Category.Group(), aggCols.Amount.Sum().As("total")).
		Where(aggCols.Category.Eq("nonexistent")).
		Scan(&rows))
	require.Empty(t, rows, "a read with no rows must clear the destination")

	// ScanOne behaves the same for a single row.
	type one struct{ Total int64 }
	got := one{Total: 999}
	require.NoError(t, database.Aggregate[*TestAggregateRecord, one](ctx).
		Select(aggCols.Amount.Sum().As("total")).
		Where(aggCols.Category.Eq("nonexistent")).
		ScanOne(&got))
	require.EqualValues(t, 0, got.Total, "the stale 999 must not survive")
}

// TestAggregateGroupByRendersRawExpression pins that the group key reaches gorm
// as an already-quoted expression it must not quote again. The MySQL, SQLite
// and PostgreSQL quoters are idempotent so a double quote is invisible there;
// the SQL Server and ClickHouse ones are not, and would emit ""col"".
func TestAggregateGroupByRendersRawExpression(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Category string
		Total    int64
	}
	statements := make([]types.SQLStatement, 0)
	rows := make([]row, 0)
	require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
		WithBuildSQL(&statements).
		Select(aggCols.Category.Group(), aggCols.Amount.Sum().As("total")).
		Scan(&rows))

	require.Len(t, statements, 1)
	require.Contains(t, statements[0].Query, "GROUP BY "+quoteIdent("category"))
	require.NotContains(t, statements[0].Query, quoteIdent(quoteIdent("category")))
}
