package database_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

// Tests for the select builder itself (select.go): its terminals, ordering
// and paging, result-row matching, dry runs and the build errors. The grouped
// side is covered in select_group_test.go, the window side in
// select_window_test.go and the constants in select_literal_test.go. They all
// read the seeded rows described at aggregateSeed in fixture_test.go, so every
// expectation is a literal a reader can check against that table by hand.

// TestSelectWithDryRun pins that WithDryRun builds the aggregate without
// executing it: no error, no database read, and the destination is left
// untouched, matching the WithDryRun contract of the Database chain.
func TestSelectWithDryRun(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Category string
		Total    int64
	}
	rows := []row{{Category: "stale", Total: 1}}
	sel := database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total"))
	require.NoError(t, sel.WithDryRun().Scan(&rows))
	require.Equal(t, []row{{Category: "stale", Total: 1}}, rows,
		"dry run loads no rows and leaves the destination unchanged")

	// The option names the next terminal alone: the builder read again runs
	// for real, the way a paginated report scans a page and counts the total.
	require.NoError(t, sel.Scan(&rows))
	require.Len(t, rows, 3)
	groups := 0
	require.NoError(t, sel.Count(&groups))
	require.Equal(t, 3, groups)
}

func TestSelectScansPointerRows(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	// A pointer row type is read through its struct on every dialect, the
	// time columns included, which sqlite reads through a stand-in struct.
	type latest struct {
		Category string
		Last     *time.Time
	}
	rows := make([]*latest, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, *latest](context.Background(), TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.OccurredAt.Max().As("last")).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Scan(&rows))
	require.Len(t, rows, 3)
	require.Equal(t, "alpha", rows[0].Category)
	require.NotNil(t, rows[0].Last)
	require.Equal(t, time.Date(2024, 1, 11, 8, 0, 0, 0, time.UTC), rows[0].Last.UTC())
}

// RowLabel is an embedded row type carrying a method, which reflect.StructOf
// cannot embed; the sqlite stand-in names it instead.
type RowLabel struct{ Note *string }

func (l RowLabel) String() string { return "label" }

// RowSpan is an embedded row type carrying a time field, embedded by value,
// through a pointer, or named under the embedded tag.
type RowSpan struct{ First time.Time }

// RowCode is a named non-struct type embedded as an anonymous field: a
// column of its own, named after the type, not an embedding to descend into.
type RowCode string

// RowMark and RowKind are named non-struct types carrying a method, unexported
// and exported, embedded as anonymous fields: columns of their own, which the
// sqlite stand-in names rather than embeds.
type RowMark string

func (m RowMark) lower() string { return strings.ToLower(string(m)) }

type RowKind string

func (k RowKind) String() string { return string(k) }

func TestSelectScansEmbeddedRowFields(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	// A time field inside an embedded struct is read on every dialect, the
	// stand-in sqlite scans through following the embedding.
	type Window struct {
		First time.Time
		Last  *time.Time
	}
	type span struct {
		Category string
		Window
	}
	rows := make([]span, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, span](context.Background(), TestAggregateRecordCols.Category.Group(),
		TestAggregateRecordCols.OccurredAt.Min().As("first"), TestAggregateRecordCols.OccurredAt.Max().As("last")).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Scan(&rows))
	require.Len(t, rows, 3)
	require.Equal(t, "alpha", rows[0].Category)
	require.Equal(t, time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC), rows[0].First.UTC())
	require.NotNil(t, rows[0].Last)
	require.Equal(t, time.Date(2024, 1, 11, 8, 0, 0, 0, time.UTC), rows[0].Last.UTC())

	// An embedded type carrying a method, placed after the time field, is
	// read the same way: the stand-in names it rather than embedding it.
	type labeled struct {
		Category string
		First    time.Time
		RowLabel
	}
	labeledRows := make([]labeled, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, labeled](context.Background(), TestAggregateRecordCols.Category.Group(),
		TestAggregateRecordCols.OccurredAt.Min().As("first"), TestAggregateRecordCols.Status.Max().As("note")).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Scan(&labeledRows))
	require.Len(t, labeledRows, 3)
	require.Equal(t, time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC), labeledRows[0].First.UTC())
	require.NotNil(t, labeledRows[0].Note)
	require.Equal(t, "failed", *labeledRows[0].Note)

	// The embedding gorm reads is followed whatever its spelling: a gorm tag
	// of the anonymous field, the embedded tag on a named field, a pointer.
	type prefixed struct {
		Category string
		First    time.Time
		RowLabel `gorm:"embeddedPrefix:p_"`
	}
	type named struct {
		Category string
		Span     RowSpan `gorm:"embedded"`
	}
	type pointed struct {
		Category string
		*RowSpan
	}
	prefixedRows := make([]prefixed, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, prefixed](context.Background(), TestAggregateRecordCols.Category.Group(),
		TestAggregateRecordCols.OccurredAt.Min().As("first"), TestAggregateRecordCols.Status.Max().As("p_note")).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Scan(&prefixedRows))
	require.Len(t, prefixedRows, 3)
	require.NotNil(t, prefixedRows[0].Note)
	require.Equal(t, "failed", *prefixedRows[0].Note)
	namedRows := make([]named, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, named](context.Background(), TestAggregateRecordCols.Category.Group(),
		TestAggregateRecordCols.OccurredAt.Min().As("first")).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Scan(&namedRows))
	require.Len(t, namedRows, 3)
	require.Equal(t, time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC), namedRows[0].Span.First.UTC())
	pointedRows := make([]pointed, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, pointed](context.Background(), TestAggregateRecordCols.Category.Group(),
		TestAggregateRecordCols.OccurredAt.Min().As("first")).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Scan(&pointedRows))
	require.Len(t, pointedRows, 3)
	require.NotNil(t, pointedRows[0].RowSpan)
	require.Equal(t, time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC), pointedRows[0].First.UTC())

	// An anonymous field of a named non-struct type is a column of its own,
	// named after the type; an unexported field is no column and is left
	// alone; a tag holding spaces is carried whole.
	type coded struct {
		RowCode
		First time.Time
	}
	codedRows := make([]coded, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, coded](context.Background(), TestAggregateRecordCols.Category.Group().As("row_code"),
		TestAggregateRecordCols.OccurredAt.Min().As("first")).
		OrderBy(TestAggregateRecordCols.Category.Group().As("row_code").Asc()).
		Scan(&codedRows))
	require.Len(t, codedRows, 3)
	require.Equal(t, RowCode("alpha"), codedRows[0].RowCode)
	require.Equal(t, time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC), codedRows[0].First.UTC())
	type partly struct {
		Category string
		First    time.Time
		hidden   int
	}
	partlyRows := make([]partly, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, partly](context.Background(), TestAggregateRecordCols.Category.Group(),
		TestAggregateRecordCols.OccurredAt.Min().As("first")).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Scan(&partlyRows))
	require.Len(t, partlyRows, 3)
	require.Equal(t, time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC), partlyRows[0].First.UTC())
	require.Zero(t, partlyRows[0].hidden)
	type spaced struct {
		Category string
		First    time.Time
		RowLabel `gorm:"embeddedPrefix:p_;comment:a note kept" json:"label"`
	}
	spacedRows := make([]spaced, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, spaced](context.Background(), TestAggregateRecordCols.Category.Group(),
		TestAggregateRecordCols.OccurredAt.Min().As("first"), TestAggregateRecordCols.Status.Max().As("p_note")).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Scan(&spacedRows))
	require.Len(t, spacedRows, 3)
	require.NotNil(t, spacedRows[0].Note)
	require.Equal(t, "failed", *spacedRows[0].Note)

	// A named non-struct type carrying methods, exported or not, is a column
	// like any other; the stand-in names it rather than embedding it.
	type marked struct {
		Category string
		First    time.Time
		RowMark
		RowKind
	}
	markedRows := make([]marked, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, marked](context.Background(), TestAggregateRecordCols.Category.Group(),
		TestAggregateRecordCols.OccurredAt.Min().As("first"), TestAggregateRecordCols.Status.Max().As("row_mark"), TestAggregateRecordCols.Status.Min().As("row_kind")).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Scan(&markedRows))
	require.Len(t, markedRows, 3)
	require.Equal(t, time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC), markedRows[0].First.UTC())
	require.Equal(t, "failed", markedRows[0].lower())
	require.Equal(t, "done", markedRows[0].RowKind.String())
}

func TestSelectWhereReusesFilters(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Total int64
	}
	got := row{}

	// A filter group is AND-combined with the rest, so the category condition
	// cannot be absorbed into the OR.
	require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Amount.Sum().As("total")).
		Where(
			TestAggregateRecordCols.Category.Eq("alpha"),
			types.FilterOr(TestAggregateRecordCols.Status.Eq("failed"), TestAggregateRecordCols.Amount.Gte(200)),
		).
		ScanOne(&got))
	// alpha rows: 100/done excluded, 200/done kept by amount, 300/failed kept.
	require.EqualValues(t, 500, got.Total)
}

func TestSelectCount(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Category string
		Total    int64
	}
	total := TestAggregateRecordCols.Amount.Sum().As("total")

	t.Run("CountsGroupsNotRows", func(t *testing.T) {
		var groups int
		require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), total).
			Count(&groups))
		require.Equal(t, 3, groups, "six rows fall into three categories")
	})

	t.Run("RespectsHaving", func(t *testing.T) {
		var groups int
		require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), total).
			Having(total.Gt(600)).
			Count(&groups))
		require.Equal(t, 1, groups)
	})

	t.Run("InnerProjectsOnlyGroupKeys", func(t *testing.T) {
		// The outer count reads nothing but how many rows the derived table
		// answers, so a measure inside it would be computed for every group
		// and then thrown away. HAVING keeps working without the measure in
		// the select list because it renders its own expression.
		var groups int
		statements := make([]types.SQLStatement, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), total).
			WithDryRun(&statements).
			Having(total.Gt(600)).
			Count(&groups))
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
		require.NoError(t, database.Select[*TestAggregateRecord, struct{ Total int64 }](context.Background(), total).
			Count(&groups))
		require.Equal(t, 1, groups)
	})
}

// TestSelectHidesSoftDeletedRows is the regression test for the failure an
// aggregate is most likely to have: it scans into a plain result row, so gorm
// parses no model and the soft-delete condition silently disappears unless the
// model is attached to the statement.
func TestSelectHidesSoftDeletedRows(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	ctx := context.Background()
	gamma := &TestAggregateRecord{ID: "a6"}
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
	require.NoError(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Amount.Sum().As("total"), types.Count().As("records")).
		ScanOne(&got))

	require.EqualValues(t, 5, got.Records, "aggregate must not count soft-deleted rows")
	require.EqualValues(t, 1500, got.Total, "2100 minus the soft-deleted 600")

	type groupRow struct {
		Category string
		Total    int64
	}
	var groups int
	require.NoError(t, database.Select[*TestAggregateRecord, groupRow](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
		Count(&groups))
	require.Equal(t, 2, groups, "the gamma group disappears with its only row")
}

func TestSelectBuildErrors(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	ctx := context.Background()
	type row struct {
		Category string
		Total    int64
	}

	t.Run("EmptyProjection", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx).Scan(&rows), database.ErrEmptyProjection)
	})

	t.Run("ColumnOfAnotherTable", func(t *testing.T) {
		// TestUser has a status column too, so the name alone would pass the
		// schema check; the table the reference carries is what refuses it.
		rows := make([]row, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), colStatus.Count().As("total")).
			Scan(&rows), database.ErrColumnTable)
	})

	t.Run("ProjectionWithoutAggregateFunction", func(t *testing.T) {
		// Group keys alone are a plain read, which List already does.
		type keyOnly struct{ Category string }
		rows := make([]keyOnly, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, keyOnly](ctx, TestAggregateRecordCols.Category.Group()).
			Scan(&rows), database.ErrPlainSelect)
	})

	t.Run("WhereOfAnotherTable", func(t *testing.T) {
		// The same mistake List refuses with ErrColumnTable: the predicate
		// fails closed for the renderer and is marked for the caller.
		rows := make([]row, 0)
		err := database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
			Where(TestRecordTagCols.Label.Eq("vip")).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrColumnTable)
		require.ErrorIs(t, err, database.ErrUnusableFilter)
		require.ErrorContains(t, err, "test_record_tags")
	})

	t.Run("WhereOfAnUnknownColumn", func(t *testing.T) {
		// A select's predicates are service code: a column the model does
		// not have fails the build rather than the statement.
		rows := make([]row, 0)
		err := database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
			Where(types.FilterEq("nosuch", "x")).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrUnusableFilter)
		require.ErrorContains(t, err, `"test_aggregate_records" does not have`)
	})

	t.Run("ConditionalMeasureOfAnotherTable", func(t *testing.T) {
		// Count leaves the measures out of its statement; the condition is
		// checked when the query is validated, so it refuses what Scan does.
		sel := database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(),
			TestAggregateRecordCols.Amount.Sum().Where(TestRecordTagCols.Label.Eq("vip")).As("total"))
		rows := make([]row, 0)
		require.ErrorIs(t, sel.Scan(&rows), database.ErrColumnTable)
		groups := 0
		require.ErrorIs(t, sel.Count(&groups), database.ErrColumnTable)
	})

	t.Run("InvalidAlias", func(t *testing.T) {
		// An alias reaches SQL as an identifier, so it is restricted to one
		// rather than quoted and hoped for.
		rows := make([]row, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("to`tal")).
			Scan(&rows), database.ErrInvalidAlias)
	})

	t.Run("TermWithoutAColumnOrAlias", func(t *testing.T) {
		// A term built by hand with neither has no name to project under.
		rows := make([]row, 0)
		err := database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), types.Term{Fn: types.FnCount}).Scan(&rows)
		require.ErrorIs(t, err, database.ErrInvalidAlias)
		require.ErrorContains(t, err, "named with As")
	})

	t.Run("UnknownColumn", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), types.NewNumericColumn[*TestAggregateRecord, int64]("nonexistent").Sum().As("total")).
			Scan(&rows), database.ErrUnknownColumn)
	})

	t.Run("SumOverNonNumericColumnViaMintedReference", func(t *testing.T) {
		// The generated reference cannot express this: Column[string] has no
		// Sum. A NumericColumn minted by hand over the column can, so the
		// build-time rule table has to stop it, otherwise MySQL answers with
		// 0 and a warning.
		rows := make([]row, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), types.NewNumericColumn[*TestAggregateRecord, string]("status").Sum().As("total")).
			Scan(&rows), database.ErrAggregateType)
	})

	t.Run("TimeBucketOverNonTimeColumn", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, types.NewTimeColumn[*TestAggregateRecord]("amount").ByDay().As("category"), TestAggregateRecordCols.Amount.Sum().As("total")).
			Scan(&rows), database.ErrAggregateType)
	})

	t.Run("DuplicateAlias", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Amount.Sum().As("total"), TestAggregateRecordCols.Amount.Max().As("total")).
			Scan(&rows), database.ErrDuplicateAlias)
	})

	t.Run("ResultRowMissingFieldForAlias", func(t *testing.T) {
		// gorm would leave the field zero and drop the column silently, which
		// reaches a report as a column of zeros.
		type missing struct{ Category string }
		rows := make([]missing, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, missing](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
			Scan(&rows), database.ErrResultFieldMissing)
	})

	t.Run("ProjectionMissingAliasForResultField", func(t *testing.T) {
		type extra struct {
			Category string
			Total    int64
			Unbound  int64
		}
		rows := make([]extra, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, extra](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
			Scan(&rows), database.ErrAliasMissing)
	})

	t.Run("HavingReferencesUnselectedMeasure", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
			Having(TestAggregateRecordCols.Score.Avg().As("avg_score").Gt(1)).
			Scan(&rows), database.ErrHavingTermNotSelected)
	})

	t.Run("UnknownAggregateFunction", func(t *testing.T) {
		// The renderer composes SQL from the constant, so a value from outside
		// the closed set would otherwise reach the statement as text.
		rows := make([]row, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), types.Term{
			Fn: "TOTALLY_NOT_SQL", Column: "amount", Alias: "total",
		}).
			Scan(&rows), database.ErrUnknownTermFn)
	})

	t.Run("UnknownTimeBucket", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, types.Term{Column: "occurred_at", Bucket: "fortnight", Alias: "category"},
			TestAggregateRecordCols.Amount.Sum().As("total")).
			Scan(&rows), database.ErrUnknownTimeBucket)
	})

	t.Run("ConditionOnGroupKey", func(t *testing.T) {
		// Conditions only restrict a measure. They used to be dropped without a
		// word, which reads as a report quietly counting the wrong rows.
		rows := make([]row, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group().Where(TestAggregateRecordCols.Status.Eq("done")),
			TestAggregateRecordCols.Amount.Sum().As("total")).
			Scan(&rows), database.ErrConditionOnGroupKey)
	})

	t.Run("BucketOnMeasure", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), types.Term{
			Fn: types.FnSum, Column: "amount", Bucket: types.TimeBucketDay, Alias: "total",
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
		require.ErrorIs(t, database.Select[*TestAggregateRecord, condRow](ctx, TestAggregateRecordCols.Category.Group(), types.Count().Where(TestAggregateRecordCols.Status.Eq("done")).As("done")).
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
		require.ErrorIs(t, database.Select[*TestAggregateRecord, flat](ctx, TestAggregateRecordCols.Category.Group(),
			TestAggregateRecordCols.Amount.Max().Where(TestAggregateRecordCols.Status.Eq("done")).As("peak")).
			Scan(&rows), database.ErrNullableResultField)
	})

	t.Run("UnusableFilterFailsFastInsteadOfEmptyReport", func(t *testing.T) {
		// A client filter that cannot be applied narrows the query. Here the
		// same predicate would turn a report into a silent zero, so it errors.
		got := struct{ Total int64 }{}
		require.ErrorIs(t, database.Select[*TestAggregateRecord, struct{ Total int64 }](ctx, TestAggregateRecordCols.Amount.Sum().As("total")).
			Where(types.FilterOr()).
			ScanOne(&got), database.ErrUnusableFilter)
	})

	t.Run("UnknownCompareOperator", func(t *testing.T) {
		rows := make([]row, 0)
		total := TestAggregateRecordCols.Amount.Sum().As("total")
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), total).
			Having(types.TermCondition{Term: total, Op: "approximately", Value: 1}).
			Scan(&rows), database.ErrUnknownCompareOp)
	})

	t.Run("UnknownOrderDirection", func(t *testing.T) {
		rows := make([]row, 0)
		total := TestAggregateRecordCols.Amount.Sum().As("total")
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), total).
			OrderBy(types.TermOrder{Term: total, Direction: "sideways"}).
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
		require.ErrorIs(t, database.Select[*TestAggregateRecord, condRow](ctx, TestAggregateRecordCols.Category.Group(), types.Count().Where(TestAggregateRecordCols.Status.Eq("done")).As("done")).
			OrderBy(types.Count().As("done").Desc()).
			Scan(&rows), database.ErrOrderTermNotSelected)
	})

	// ScanOne rejects all three pagination inputs, not just the one that
	// happened to be covered.
	for name, build := range map[string]func(types.Selector[*TestAggregateRecord, struct{ Total int64 }]) types.Selector[*TestAggregateRecord, struct{ Total int64 }]{
		"Limit": func(a types.Selector[*TestAggregateRecord, struct{ Total int64 }]) types.Selector[*TestAggregateRecord, struct{ Total int64 }] {
			return a.Limit(1)
		},
		"Page": func(a types.Selector[*TestAggregateRecord, struct{ Total int64 }]) types.Selector[*TestAggregateRecord, struct{ Total int64 }] {
			return a.Page(2, 1)
		},
		"Having": func(a types.Selector[*TestAggregateRecord, struct{ Total int64 }]) types.Selector[*TestAggregateRecord, struct{ Total int64 }] {
			return a.Having(TestAggregateRecordCols.Amount.Sum().As("total").Gt(1))
		},
	} {
		t.Run("ScanOneRejects"+name, func(t *testing.T) {
			got := struct{ Total int64 }{}
			base := database.Select[*TestAggregateRecord, struct{ Total int64 }](ctx, TestAggregateRecordCols.Amount.Sum().As("total"))
			require.ErrorIs(t, build(base).ScanOne(&got), database.ErrScanOnePaged)
		})
	}

	t.Run("ScanOneRejectsGroupedQuery", func(t *testing.T) {
		got := row{}
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
			ScanOne(&got), database.ErrGroupedScanOne)
	})
}

func TestSelectWithDryRunCollector(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Category string
		Total    int64
	}
	rows := make([]row, 0)
	statements := make([]types.SQLStatement, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
		WithDryRun(&statements).
		Scan(&rows))

	require.Len(t, statements, 1)
	sql := statements[0].RenderedSQL
	require.Contains(t, sql, "COALESCE(SUM(")
	require.Contains(t, sql, "GROUP BY")
	require.Contains(t, sql, "deleted_at")
	require.Empty(t, rows, "dry run builds SQL without reading rows")
}

// TestSelectBuilderReuse pins the paginated-report idiom: read the page,
// then count the groups off the same builder. The chain's gorm session keeps
// the clauses of whatever ran on it before, so without a fresh statement per
// build the count inherits the page's LIMIT and reports the page size as the
// total -- silently, and only when tracing is off.
func TestSelectBuilderReuse(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	type row struct {
		Category string
		Total    int64
	}
	ctx := context.Background()
	builder := database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
		Where(TestAggregateRecordCols.Status.Eq("done")).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Limit(1)

	rows := make([]row, 0)
	require.NoError(t, builder.Scan(&rows))
	require.Equal(t, []row{{Category: "alpha", Total: 300}}, rows)

	var groups int
	require.NoError(t, builder.Count(&groups))
	require.Equal(t, 3, groups, "the page limit must not leak into the total")

	// A second identical read repeats the query rather than compounding it.
	again := make([]row, 0)
	require.NoError(t, builder.Scan(&again))
	require.Equal(t, rows, again)
}

// TestSelectNullableResultFields pins where a result field must be able to
// tell NULL apart from zero. AVG, MIN and MAX return NULL when they see no
// value, so the field needs a pointer or a sql.Null wrapper — except for a
// grouped, unconditional measure over a non-nullable column, where every group
// holds at least one real value and a plain field cannot receive NULL.
func TestSelectNullableResultFields(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	ctx := context.Background()

	t.Run("AcceptsSQLNullWrappers", func(t *testing.T) {
		type row struct {
			Peak     sql.NullInt64
			AvgScore sql.NullFloat64
		}
		got := row{}
		require.NoError(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Amount.Max().As("peak"), TestAggregateRecordCols.Score.Avg().As("avg_score")).
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
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Amount.Max().As("peak")).
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
		require.NoError(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(),
			TestAggregateRecordCols.Amount.Max().As("peak"),
			TestAggregateRecordCols.Amount.Min().As("earliest"),
			TestAggregateRecordCols.Score.Avg().As("avg_score")).
			OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
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
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.ClosedAt.Max().As("last_seen")).
			Scan(&rows), database.ErrNullableResultField)
		// With the rows without a closed_at kept out in Where the MAX is never
		// NULL, and the plain field is accepted.
		require.NoError(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.ClosedAt.Max().As("last_seen")).
			Where(TestAggregateRecordCols.ClosedAt.IsNotNull()).
			Scan(&rows))
	})

	t.Run("NullableGroupKeyRejectsPlainField", func(t *testing.T) {
		// The rows without a closed_at form a group of their own, keyed
		// NULL, and a bucket of NULL is NULL as well; a pointer field reads
		// the group, nil for those rows.
		type keyed struct {
			ClosedAt time.Time
			Records  int64
		}
		rows := make([]keyed, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, keyed](ctx, TestAggregateRecordCols.ClosedAt.Group(), types.Count().As("records")).
			Scan(&rows), database.ErrNullableResultField)
		type bucketed struct {
			Day     string
			Records int64
		}
		buckets := make([]bucketed, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, bucketed](ctx, TestAggregateRecordCols.ClosedAt.ByDay().As("day"), types.Count().As("records")).
			Scan(&buckets), database.ErrNullableResultField)
		type pointed struct {
			ClosedAt *time.Time
			Records  int64
		}
		groups := make([]pointed, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, pointed](ctx, TestAggregateRecordCols.ClosedAt.Group(), types.Count().As("records")).
			Where(TestAggregateRecordCols.Category.Eq("beta")).
			Scan(&groups))
		require.Len(t, groups, 1)
		require.Nil(t, groups[0].ClosedAt)
		require.EqualValues(t, 2, groups[0].Records)
		// A condition on the column in Where keeps the rows without one out,
		// and the plain field is accepted; inside an OR group it need not
		// hold, and the field stays a pointer.
		require.NoError(t, database.Select[*TestAggregateRecord, keyed](ctx, TestAggregateRecordCols.ClosedAt.Group(), types.Count().As("records")).
			Where(TestAggregateRecordCols.ClosedAt.IsNotNull()).
			Scan(&rows))
		require.NoError(t, database.Select[*TestAggregateRecord, bucketed](ctx, TestAggregateRecordCols.ClosedAt.ByDay().As("day"), types.Count().As("records")).
			Where(TestAggregateRecordCols.ClosedAt.Gte(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))).
			Scan(&buckets))
		require.ErrorIs(t, database.Select[*TestAggregateRecord, keyed](ctx, TestAggregateRecordCols.ClosedAt.Group(), types.Count().As("records")).
			Where(types.FilterOr(TestAggregateRecordCols.ClosedAt.IsNotNull(), TestAggregateRecordCols.Category.Eq("beta"))).
			Scan(&rows), database.ErrNullableResultField)
	})

	t.Run("ColumnTheSchemaForbidsNullOnNeedsNoPointer", func(t *testing.T) {
		// A pointer column under a NOT NULL constraint never holds NULL: its
		// group key and its MAX read into plain fields.
		defer func() { _ = database.DB().Exec("DELETE FROM test_marked_records").Error }()
		require.NoError(t, database.Database[*TestMarkedRecord](ctx).Create(
			&TestMarkedRecord{Mark: new("m1"), Amount: 1},
			&TestMarkedRecord{Mark: new("m1"), Amount: 2},
			&TestMarkedRecord{Mark: new("m2"), Amount: 3},
		))
		type row struct {
			Mark  string
			Total int64
		}
		rows := make([]row, 0)
		require.NoError(t, database.Select[*TestMarkedRecord, row](ctx, TestMarkedRecordCols.Mark.Group(), TestMarkedRecordCols.Amount.Sum().As("total")).
			OrderBy(TestMarkedRecordCols.Mark.Group().Asc()).
			Scan(&rows))
		require.Equal(t, []row{{Mark: "m1", Total: 3}, {Mark: "m2", Total: 3}}, rows)
		type peak struct {
			Amount int64
			Top    string
		}
		peaks := make([]peak, 0)
		require.NoError(t, database.Select[*TestMarkedRecord, peak](ctx, TestMarkedRecordCols.Amount.Group(), TestMarkedRecordCols.Mark.Max().As("top")).
			OrderBy(TestMarkedRecordCols.Amount.Group().Asc()).
			Scan(&peaks))
		require.Equal(t, []peak{{Amount: 1, Top: "m1"}, {Amount: 2, Top: "m1"}, {Amount: 3, Top: "m2"}}, peaks)
	})

	t.Run("AcceptsPointerFields", func(t *testing.T) {
		// A pointer field is the other accepted shape besides the sql.Null
		// wrappers: it carries the value for a group that has one and stays
		// nil for a group holding only NULLs, so the two outcomes remain
		// distinguishable. Give one alpha row a closed_at so both shapes
		// appear in a single read.
		require.NoError(t, database.DB().Exec(
			"UPDATE test_aggregate_records SET closed_at = occurred_at WHERE id = 'a1'",
		).Error)

		type row struct {
			Category string
			LastSeen *time.Time
		}
		rows := make([]row, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.ClosedAt.Max().As("last_seen")).
			OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
			Scan(&rows))
		require.Len(t, rows, 3)
		require.Equal(t, "alpha", rows[0].Category)
		require.NotNil(t, rows[0].LastSeen)
		require.Equal(t, "beta", rows[1].Category)
		require.Nil(t, rows[1].LastSeen)
		require.Equal(t, "gamma", rows[2].Category)
		require.Nil(t, rows[2].LastSeen)
	})

	t.Run("AcceptsNullTimeField", func(t *testing.T) {
		// The same read through the other accepted shape: sql.NullTime tells
		// absence apart through Valid. The update is the one AcceptsPointerFields
		// runs, repeated so the case stands on its own.
		require.NoError(t, database.DB().Exec(
			"UPDATE test_aggregate_records SET closed_at = occurred_at WHERE id = 'a1'",
		).Error)

		type row struct {
			Category string
			LastSeen sql.NullTime
		}
		rows := make([]row, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.ClosedAt.Max().As("last_seen")).
			OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
			Scan(&rows))
		require.Len(t, rows, 3)
		require.True(t, rows[0].LastSeen.Valid)
		require.False(t, rows[1].LastSeen.Valid)
		require.False(t, rows[2].LastSeen.Valid)
	})

	t.Run("PlainTimeOverNonNullableColumn", func(t *testing.T) {
		// A grouped, unconditional measure over a non-nullable column keeps a
		// plain time field, and the value round-trips exactly on every
		// dialect, including the sqlite TEXT detour.
		type row struct {
			Category string
			LastSeen time.Time
		}
		rows := make([]row, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.OccurredAt.Max().As("last_seen")).
			OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
			Scan(&rows))
		require.Len(t, rows, 3)
		expected := []struct {
			category string
			lastSeen time.Time
		}{
			{"alpha", time.Date(2024, 1, 11, 8, 0, 0, 0, time.UTC)},
			{"beta", time.Date(2024, 2, 10, 8, 0, 0, 0, time.UTC)},
			{"gamma", time.Date(2024, 2, 11, 10, 0, 0, 0, time.UTC)},
		}
		for i, want := range expected {
			require.Equal(t, want.category, rows[i].Category)
			// The instant is what round-trips; the Location a driver hands it
			// back in differs per dialect, so compare with EqCol, not ==.
			require.True(t, rows[i].LastSeen.Equal(want.lastSeen),
				"category %s: got %s, want %s", want.category, rows[i].LastSeen, want.lastSeen)
		}
	})
}

// TestSelectScanReplacesDest pins that a read replaces the destination
// rather than appending to it. gorm keeps the existing elements when a scan
// returns no rows, so a reused destination would still hold the previous
// result and the caller would read a stale report as a fresh one.
func TestSelectScanReplacesDest(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	ctx := context.Background()
	type row struct {
		Category string
		Total    int64
	}
	rows := make([]row, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
		Scan(&rows))
	require.Len(t, rows, 3)

	// A second read matching nothing must empty it, not leave the first result.
	require.NoError(t, database.Select[*TestAggregateRecord, row](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
		Where(TestAggregateRecordCols.Category.Eq("nonexistent")).
		Scan(&rows))
	require.Empty(t, rows, "a read with no rows must clear the destination")

	// ScanOne behaves the same for a single row.
	type one struct{ Total int64 }
	got := one{Total: 999}
	require.NoError(t, database.Select[*TestAggregateRecord, one](ctx, TestAggregateRecordCols.Amount.Sum().As("total")).
		Where(TestAggregateRecordCols.Category.Eq("nonexistent")).
		ScanOne(&got))
	require.EqualValues(t, 0, got.Total, "the stale 999 must not survive")
}
