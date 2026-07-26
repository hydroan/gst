package database_test

import (
	"context"
	"testing"
	"time"

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
}{
	Category:   types.Column[string]{Name: "category"},
	Status:     types.Column[string]{Name: "status"},
	Amount:     types.NumericColumn[int64]{Column: types.Column[int64]{Name: "amount"}},
	Score:      types.NumericColumn[float64]{Column: types.Column[float64]{Name: "score"}},
	OccurredAt: types.TimeColumn{Column: types.Column[time.Time]{Name: "occurred_at"}},
}

func TestAggregateScalar(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Total    int64
		Records  int64
		Smallest int64
		Largest  int64
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
	require.EqualValues(t, 100, got.Smallest)
	require.EqualValues(t, 600, got.Largest)
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

	t.Run("OrderByMeasureWithLimit", func(t *testing.T) {
		rows := make([]row, 0)
		require.NoError(t, database.Aggregate[*TestAggregateRecord, row](context.Background()).
			Select(aggCols.Category.Group(), total).
			OrderBy(total.Desc()).
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
			Scan(&rows), database.ErrAliasMissing)
	})

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
	ID:       types.Column[string]{Name: "id"},
	RecordID: types.Column[string]{Name: "record_id"},
	Label:    types.Column[string]{Name: "label"},
}

var recordIDCol = types.Column[string]{Name: "id"}

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
