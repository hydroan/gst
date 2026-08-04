package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/database/clickhouse"
	"github.com/hydroan/gst/internal/testcontainer"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

// TestClickhouse covers the contract of an application-held clickhouse
// instance: the read and aggregate paths work, and the write path, the
// transaction boundary and row locks fail fast per the capability-miss rule.
//
// The container, the table and the seed are prepared once for all subtests.
// The table is created with native DDL: clickhouse schema (engine, ORDER BY)
// is owned by the application's ingestion side, which is also why the seed
// arrives through a plain INSERT rather than the framework write path. The
// seed mirrors the aggregate fixture, so expectations read the same as in
// the aggregate tests.
func TestClickhouse(t *testing.T) {
	cfg, terminate, err := testcontainer.SetupClickhouse()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, terminate()) })

	ins, err := clickhouse.New(cfg)
	require.NoError(t, err)

	require.NoError(t, ins.Exec(`CREATE TABLE test_aggregate_records (
		id          String,
		created_by  String,
		updated_by  String,
		created_at  DateTime64(3),
		updated_at  DateTime64(3),
		deleted_at  Nullable(DateTime64(3)),
		category    String,
		status      String,
		amount      Int64,
		score       Float64,
		occurred_at DateTime64(3),
		closed_at   Nullable(DateTime64(3))
	) ENGINE = MergeTree ORDER BY (category, occurred_at)`).Error)

	require.NoError(t, ins.Exec(`INSERT INTO test_aggregate_records
		(id, category, status, amount, score, occurred_at, created_at, updated_at) VALUES
		('a1','alpha','done',  100,1.5,'2024-01-10 08:00:00','2024-01-01 00:00:00','2024-01-01 00:00:00'),
		('a2','alpha','done',  200,2.5,'2024-01-10 09:00:00','2024-01-01 00:00:00','2024-01-01 00:00:00'),
		('a3','alpha','failed',300,3.5,'2024-01-11 08:00:00','2024-01-01 00:00:00','2024-01-01 00:00:00'),
		('a4','beta', 'done',  400,4.5,'2024-02-10 08:00:00','2024-01-01 00:00:00','2024-01-01 00:00:00'),
		('a5','beta', 'failed',500,5.5,'2024-02-10 08:00:00','2024-01-01 00:00:00','2024-01-01 00:00:00'),
		('a6','gamma','done',  600,6.5,'2024-02-11 10:00:00','2024-01-01 00:00:00','2024-01-01 00:00:00')`).Error)

	ctx := context.Background()

	listIDs := func(t *testing.T, filters ...types.Filter) []string {
		t.Helper()
		records := make([]*TestAggregateRecord, 0)
		require.NoError(t, database.DatabaseOn[*TestAggregateRecord](ctx, ins).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true, Filters: filters}).
			WithOrder(types.Asc("id")).
			List(&records))
		ids := make([]string, 0, len(records))
		for _, r := range records {
			ids = append(ids, r.ID)
		}
		return ids
	}

	t.Run("ListWithFiltersOrderAndPaging", func(t *testing.T) {
		require.Equal(t, []string{"a4", "a5", "a6"}, listIDs(t, types.FilterGte("amount", 400)))
		require.Equal(t, []string{"a1", "a2", "a3"}, listIDs(t, types.FilterIn("category", []string{"alpha"})))
		require.Equal(t, []string{"a1", "a2"}, listIDs(t, types.FilterLike("category", "alph"), types.FilterEq("status", "done")))

		page := make([]*TestAggregateRecord, 0)
		require.NoError(t, database.DatabaseOn[*TestAggregateRecord](ctx, ins).
			WithQuery(nil, types.QueryOptions{AllowEmpty: true}).
			WithOrder(types.Desc("amount")).
			WithLimit(2).
			List(&page))
		require.Len(t, page, 2)
		require.Equal(t, "a6", page[0].ID)
		require.Equal(t, "a5", page[1].ID)
	})

	t.Run("TimeFilterWithBothValueShapes", func(t *testing.T) {
		boundary := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		require.Equal(t, []string{"a4", "a5", "a6"},
			listIDs(t, types.FilterGte("occurred_at", boundary.Format(types.FilterTimeLayout))))
		require.Equal(t, []string{"a1", "a2", "a3"}, listIDs(t, types.FilterLt("occurred_at", boundary)))
	})

	t.Run("CountRows", func(t *testing.T) {
		var count int
		require.NoError(t, database.DatabaseOn[*TestAggregateRecord](ctx, ins).Count(&count))
		require.Equal(t, 6, count)
	})

	t.Run("CursorPagesOnTime", func(t *testing.T) {
		page := make([]*TestAggregateRecord, 0)
		boundary := time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC)
		require.NoError(t, database.DatabaseOn[*TestAggregateRecord](ctx, ins).
			WithCursor(types.CursorForward(types.Asc("occurred_at"), boundary.Format(types.FilterTimeLayout))).
			WithLimit(1).
			List(&page))
		require.Len(t, page, 1)
		require.Equal(t, "a2", page[0].ID, "the boundary row itself must not leak back into the page")
	})

	t.Run("AggregateGroupsMeasuresAndHaving", func(t *testing.T) {
		type row struct {
			Category string
			Total    int64
			Done     int64
		}
		rows := make([]row, 0)
		require.NoError(t, database.AggregateOn[*TestAggregateRecord, row](ctx, ins).
			Select(
				aggCols.Category.Group(),
				aggCols.Amount.Sum().As("total"),
				types.Count().Where(aggCols.Status.Eq("done")).As("done"),
			).
			Having(aggCols.Amount.Sum().As("total").Gte(600)).
			OrderBy(aggCols.Category.Group().Asc()).
			Scan(&rows))
		require.Equal(t, []row{
			{Category: "alpha", Total: 600, Done: 2},
			{Category: "beta", Total: 900, Done: 1},
			{Category: "gamma", Total: 600, Done: 1},
		}, rows)
	})

	t.Run("AggregateTimeBuckets", func(t *testing.T) {
		type row struct {
			Bucket  string
			Records int64
		}
		rows := make([]row, 0)
		require.NoError(t, database.AggregateOn[*TestAggregateRecord, row](ctx, ins).
			Select(aggCols.OccurredAt.ByDay().As("bucket"), types.Count().As("records")).
			OrderBy(aggCols.OccurredAt.ByDay().As("bucket").Asc()).
			Scan(&rows))
		require.Equal(t, []row{
			{Bucket: "2024-01-10", Records: 2},
			{Bucket: "2024-01-11", Records: 1},
			{Bucket: "2024-02-10", Records: 2},
			{Bucket: "2024-02-11", Records: 1},
		}, rows)
	})

	t.Run("CountGroups", func(t *testing.T) {
		type row struct {
			Category string
			Total    int64
		}
		var groups int
		require.NoError(t, database.AggregateOn[*TestAggregateRecord, row](ctx, ins).
			Select(aggCols.Category.Group(), aggCols.Amount.Sum().As("total")).
			CountGroups(&groups))
		require.Equal(t, 3, groups)
	})

	// The capability-miss rule: everything below is not carried by clickhouse
	// and must answer ErrUnsupportedOnDialect instead of half-working.
	t.Run("WriteTransactionAndLockFailFast", func(t *testing.T) {
		record := &TestAggregateRecord{Category: "x", Base: model.Base{ID: "w1"}}

		require.ErrorIs(t, database.DatabaseOn[*TestAggregateRecord](ctx, ins).Create(record), database.ErrUnsupportedOnDialect)
		require.ErrorIs(t, database.DatabaseOn[*TestAggregateRecord](ctx, ins).Update(record), database.ErrUnsupportedOnDialect)
		require.ErrorIs(t, database.DatabaseOn[*TestAggregateRecord](ctx, ins).Delete(record), database.ErrUnsupportedOnDialect)
		require.ErrorIs(t, database.DatabaseOn[*TestAggregateRecord](ctx, ins).Upsert(record), database.ErrUnsupportedOnDialect)
		require.ErrorIs(t, database.DatabaseOn[*TestAggregateRecord](ctx, ins).UpdateByID("a1", "status", "x"), database.ErrUnsupportedOnDialect)
		require.ErrorIs(t, database.DatabaseOn[*TestAggregateRecord](ctx, ins).Cleanup(), database.ErrUnsupportedOnDialect)
		require.ErrorIs(t, database.DatabaseOn[*TestAggregateRecord](ctx, ins).WithBuildSQL(&[]types.SQLStatement{}).Create(record), database.ErrUnsupportedOnDialect,
			"the dry-run path is rejected the same way")

		require.ErrorIs(t, database.TransactionOn(ctx, ins, func(ctx context.Context) error { return nil }), database.ErrUnsupportedOnDialect)

		got := new(TestAggregateRecord)
		require.ErrorIs(t, database.DatabaseOn[*TestAggregateRecord](ctx, ins).WithLock().Get(got, "a1"), database.ErrUnsupportedOnDialect)

		var count int
		require.NoError(t, database.DatabaseOn[*TestAggregateRecord](ctx, ins).Count(&count))
		require.Equal(t, 6, count, "rejected writes must leave the table untouched")
	})

	t.Run("JSONContainsFailsClosed", func(t *testing.T) {
		// datatypes has no clickhouse arm, so the filter fails closed to an
		// empty result instead of rendering an empty condition that would
		// silently widen it.
		require.Empty(t, listIDs(t, types.Filter{Column: "category", Op: types.FilterOpJSONContains, Value: "alpha"}))
	})

	t.Run("RegexFilter", func(t *testing.T) {
		// ClickHouse parses the REGEXP operator natively, so the regex filters
		// need no special arm.
		require.Equal(t, []string{"a1", "a2", "a3"}, listIDs(t, types.FilterRegex("category", "^al.*")))
	})

	t.Run("ExistsSubqueryFailsClosed", func(t *testing.T) {
		// ClickHouse cannot resolve a correlated column from the enclosing
		// query, so the semi join fails closed: even a tag row that would
		// match on a capable dialect selects nothing here.
		require.NoError(t, ins.Exec(`CREATE TABLE test_record_tags (
			id         String,
			created_by String,
			updated_by String,
			created_at DateTime64(3),
			updated_at DateTime64(3),
			deleted_at Nullable(DateTime64(3)),
			record_id  String,
			label      String
		) ENGINE = MergeTree ORDER BY (record_id)`).Error)
		require.NoError(t, ins.Exec(`INSERT INTO test_record_tags (id, record_id, label, created_at, updated_at) VALUES
			('t1','a1','vip','2024-01-01 00:00:00','2024-01-01 00:00:00')`).Error)

		require.Empty(t, listIDs(t, types.FilterExists[*TestRecordTag](
			tagCols.RecordID, recordIDCol,
		)))
	})
}
