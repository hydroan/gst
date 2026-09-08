package database_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

// The union tests stack the six seeded records (see aggregateSeed in
// fixture_test.go) with the three seeded tags (see tagSeed): t1 and t2 sit on
// alpha records, t3 on a beta record. Every expectation below is a literal a
// reader can check against those two tables by hand.

// feedRow is the row type the record and tag branches stack into.
type feedRow struct {
	Kind     string
	ID       string
	Category string
}

// recordsBranch projects every record as a feed row tagged "record".
func recordsBranch(ctx context.Context) types.Selector[*TestAggregateRecord, feedRow] {
	return database.Select[*TestAggregateRecord, feedRow](ctx,
		types.Literal("record").As("kind"), TestAggregateRecordCols.ID, TestAggregateRecordCols.Category)
}

// tagsBranch projects every tag as a feed row tagged "tag". It declares its
// terms in another order than feedRow on purpose: the framework renders every
// member's SELECT list in the result row's order, so the declaration order
// never decides how the columns line up.
func tagsBranch(ctx context.Context) types.Selector[*TestRecordTag, feedRow] {
	return database.Select[*TestRecordTag, feedRow](ctx,
		TestRecordTagCols.Category, TestRecordTagCols.ID, types.Literal("tag").As("kind"))
}

func setupUnionData(t *testing.T) {
	t.Helper()
	setupAggregateData(t)
	setupTagData(t)
}

func cleanupUnionData() {
	cleanupAggregateData()
	cleanupTagData()
}

// feedMember renders the SELECT a member of the feed reads, in feedRow's
// order, over the given table.
func feedMember(kind, table string) string {
	return "SELECT '" + kind + "' AS " + quoteIdent("kind") + ", " + quoteIdent("id") + " AS " + quoteIdent("id") +
		", " + quoteIdent("category") + " AS " + quoteIdent("category") +
		" FROM " + quoteIdent(table) + " WHERE " + quoteIdent(table) + "." + quoteIdent("deleted_at") + " IS NULL"
}

func TestUnionAllDryRunAppliesToTheNextTerminalOnly(t *testing.T) {
	defer cleanupUnionData()
	setupUnionData(t)
	ctx := context.Background()

	feed := database.UnionAll[feedRow](ctx, recordsBranch(ctx), tagsBranch(ctx))
	statements := make([]types.SQLStatement, 0)
	rows := make([]feedRow, 0)
	require.NoError(t, feed.WithDryRun(&statements).Scan(&rows))
	require.Len(t, statements, 1)
	require.Empty(t, rows)

	// The union read again runs for real: a page and its total from one
	// builder, the dry run consumed by the terminal it was set for.
	require.NoError(t, feed.Scan(&rows))
	require.Len(t, rows, 9)
	total := 0
	require.NoError(t, feed.Count(&total))
	require.Equal(t, 9, total)

	// A member's own dry run is the member's: the union's terminal runs the
	// member for real and consumes the option, so the member read again on
	// its own executes as well.
	type categoryCount struct {
		Category string
		N        int64
	}
	perCategory := database.Select[*TestAggregateRecord, categoryCount](ctx, TestAggregateRecordCols.Category.Group(), types.Count().As("n"))
	perLabel := database.Select[*TestRecordTag, categoryCount](ctx, TestRecordTagCols.Label.Group().As("category"), types.Count().As("n"))
	statements = statements[:0]
	counts := make([]categoryCount, 0)
	require.NoError(t, database.UnionAll[categoryCount](ctx, perCategory.WithDryRun(&statements), perLabel).Scan(&counts))
	require.Len(t, counts, 5)
	require.Empty(t, statements)
	require.NoError(t, perCategory.Scan(&counts))
	require.Len(t, counts, 3)
}

func TestUnionAllStacksBranches(t *testing.T) {
	defer cleanupUnionData()
	setupUnionData(t)
	ctx := context.Background()

	t.Run("StacksTheRowsOfEveryBranch", func(t *testing.T) {
		rows := make([]feedRow, 0)
		// The ordering names result columns; a column reference of either
		// model does, as long as its name is one.
		require.NoError(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx), tagsBranch(ctx)).
			OrderBy(TestAggregateRecordCols.Category.Asc(), TestRecordTagCols.ID.Asc()).
			Scan(&rows))
		require.Equal(t, []feedRow{
			{Kind: "record", ID: "a1", Category: "alpha"},
			{Kind: "record", ID: "a2", Category: "alpha"},
			{Kind: "record", ID: "a3", Category: "alpha"},
			{Kind: "tag", ID: "t1", Category: "alpha"},
			{Kind: "tag", ID: "t2", Category: "alpha"},
			{Kind: "record", ID: "a4", Category: "beta"},
			{Kind: "record", ID: "a5", Category: "beta"},
			{Kind: "tag", ID: "t3", Category: "beta"},
			{Kind: "record", ID: "a6", Category: "gamma"},
		}, rows)
	})

	t.Run("RendersEveryMemberInTheResultRowOrder", func(t *testing.T) {
		statements := make([]types.SQLStatement, 0)
		rows := make([]feedRow, 0)
		require.NoError(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx), tagsBranch(ctx)).
			OrderBy(TestAggregateRecordCols.Category.Asc(), TestAggregateRecordCols.ID.Asc()).
			WithDryRun(&statements).Scan(&rows))
		require.Len(t, statements, 1)
		require.Equal(t,
			"SELECT * FROM (SELECT * FROM ("+feedMember("record", "test_aggregate_records")+") AS "+quoteIdent("b0")+
				" UNION ALL SELECT * FROM ("+feedMember("tag", "test_record_tags")+") AS "+quoteIdent("b1")+") AS "+quoteIdent("u")+
				" ORDER BY "+quoteIdent("category")+" ASC,"+quoteIdent("id")+" ASC",
			statements[0].Query,
			"every member is a derived table spelling its SELECT list in feedRow's order, and the union is one too")
		require.Empty(t, statements[0].Args, "without a limit nothing is pushed down and nothing binds")
	})

	t.Run("ReplacesTheDestination", func(t *testing.T) {
		rows := []feedRow{{Kind: "stale"}}
		require.NoError(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx).Where(TestAggregateRecordCols.Category.Eq("gamma"))).
			Scan(&rows))
		require.Equal(t, []feedRow{{Kind: "record", ID: "a6", Category: "gamma"}}, rows)
	})
}

func TestUnionAllPushesOrderAndLimitIntoBranches(t *testing.T) {
	defer cleanupUnionData()
	setupUnionData(t)
	ctx := context.Background()
	feed := func() types.Union[feedRow] {
		return database.UnionAll[feedRow](ctx, recordsBranch(ctx), tagsBranch(ctx)).
			OrderBy(TestAggregateRecordCols.Category.Asc(), TestAggregateRecordCols.ID.Asc())
	}

	t.Run("PagesTheStackedRows", func(t *testing.T) {
		// Under (category, id) the stack reads a1 a2 a3 t1 t2 a4 a5 t3 a6;
		// the page skips three and takes two.
		rows := make([]feedRow, 0)
		require.NoError(t, feed().Limit(2).Offset(3).Scan(&rows))
		require.Equal(t, []feedRow{
			{Kind: "tag", ID: "t1", Category: "alpha"},
			{Kind: "tag", ID: "t2", Category: "alpha"},
		}, rows)
	})

	t.Run("RendersTheOrderAndTheCapInEveryMember", func(t *testing.T) {
		statements := make([]types.SQLStatement, 0)
		rows := make([]feedRow, 0)
		require.NoError(t, feed().Limit(2).Offset(3).WithDryRun(&statements).Scan(&rows))
		require.Len(t, statements, 1)
		sql := statements[0].Query
		ordered := "ORDER BY " + quoteIdent("category") + " ASC," + quoteIdent("id") + " ASC LIMIT "
		require.Equal(t, 3, strings.Count(sql, ordered),
			"both members and the union carry the ordering and a limit")
		require.Contains(t, sql, " IS NULL "+ordered)
		require.Equal(t, 1, strings.Count(sql, "OFFSET "), "the offset applies to the stacked rows alone")
		rendered := statements[0].RenderedSQL
		require.Equal(t, 2, strings.Count(rendered, " LIMIT 5) AS "+quoteIdent("b")[:1]),
			"each member reads offset plus limit rows")
		require.Contains(t, rendered, ") AS "+quoteIdent("u")+" "+ordered+"2 OFFSET 3", "the union pages the stacked rows")
	})

	t.Run("PushesTheCapAloneWithoutAnOrder", func(t *testing.T) {
		statements := make([]types.SQLStatement, 0)
		rows := make([]feedRow, 0)
		require.NoError(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx), tagsBranch(ctx)).
			Limit(4).WithDryRun(&statements).Scan(&rows))
		require.Len(t, statements, 1)
		require.NotContains(t, statements[0].Query, "ORDER BY")
		require.Equal(t, 3, strings.Count(statements[0].RenderedSQL, " LIMIT 4"), "both members and the union are capped")
	})

	t.Run("PushesNothingWithoutALimit", func(t *testing.T) {
		statements := make([]types.SQLStatement, 0)
		rows := make([]feedRow, 0)
		require.NoError(t, feed().WithDryRun(&statements).Scan(&rows))
		require.Len(t, statements, 1)
		require.Equal(t, 1, strings.Count(statements[0].Query, "ORDER BY"),
			"an unpaged union sorts the stack once, ordering the members would be wasted work")
		require.Empty(t, statements[0].Args)
	})
}

func TestUnionAllStacksProjectionShapes(t *testing.T) {
	defer cleanupUnionData()
	setupUnionData(t)
	ctx := context.Background()

	t.Run("GroupedBesidePlain", func(t *testing.T) {
		type row struct {
			Kind     string
			Category string
			Amount   int64
		}
		// kind is shared with the totals branch so the union can order by it:
		// a term ordering names the term a branch projects.
		kind := types.Literal("total").As("kind")
		totals := database.Select[*TestAggregateRecord, row](ctx,
			kind, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum())
		each := database.Select[*TestAggregateRecord, row](ctx,
			types.Literal("row").As("kind"), TestAggregateRecordCols.Category, TestAggregateRecordCols.Amount)

		rows := make([]row, 0)
		require.NoError(t, database.UnionAll[row](ctx, totals, each).
			OrderBy(TestAggregateRecordCols.Category.Asc(), kind.Desc(), TestAggregateRecordCols.Amount.Asc()).
			Scan(&rows))
		require.Equal(t, []row{
			{Kind: "total", Category: "alpha", Amount: 600},
			{Kind: "row", Category: "alpha", Amount: 100},
			{Kind: "row", Category: "alpha", Amount: 200},
			{Kind: "row", Category: "alpha", Amount: 300},
			{Kind: "total", Category: "beta", Amount: 900},
			{Kind: "row", Category: "beta", Amount: 400},
			{Kind: "row", Category: "beta", Amount: 500},
			{Kind: "total", Category: "gamma", Amount: 600},
			{Kind: "row", Category: "gamma", Amount: 600},
		}, rows)

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, database.UnionAll[row](ctx, totals, each).WithDryRun(&statements).Scan(&rows))
		require.Len(t, statements, 1)
		require.Contains(t, statements[0].Query,
			"COALESCE(SUM("+quoteIdent("amount")+"), 0) AS "+quoteIdent("amount")+" FROM "+quoteIdent("test_aggregate_records")+
				" WHERE "+quoteIdent("test_aggregate_records")+"."+quoteIdent("deleted_at")+" IS NULL GROUP BY "+quoteIdent("category")+") AS "+quoteIdent("b0"),
			"a grouped member keeps its GROUP BY, and the literal stays out of it")
	})

	t.Run("QualifiedBesideWindowed", func(t *testing.T) {
		type row struct {
			Kind     string
			ID       string
			Category string
			Rn       int64
		}
		latestRn := types.RowNumber().
			Over(types.PartitionBy(TestAggregateRecordCols.Category).OrderBy(TestAggregateRecordCols.OccurredAt.Desc())).
			As("rn")
		latest := database.Select[*TestAggregateRecord, row](ctx,
			types.Literal("latest").As("kind"), TestAggregateRecordCols.ID, TestAggregateRecordCols.Category, latestRn).
			Qualify(latestRn.Eq(1))
		tagRn := types.RowNumber().Over(types.OrderBy(TestRecordTagCols.ID.Asc())).As("rn")
		tags := database.Select[*TestRecordTag, row](ctx,
			types.Literal("tag").As("kind"), TestRecordTagCols.ID, TestRecordTagCols.Category, tagRn)

		// Under (category, id): a3 t1 t2 a4 t3 a6; the page takes four.
		rows := make([]row, 0)
		feed := database.UnionAll[row](ctx, latest, tags).
			OrderBy(TestAggregateRecordCols.Category.Asc(), TestAggregateRecordCols.ID.Asc()).
			Limit(4)
		require.NoError(t, feed.Scan(&rows))
		require.Equal(t, []row{
			{Kind: "latest", ID: "a3", Category: "alpha", Rn: 1},
			{Kind: "tag", ID: "t1", Category: "alpha", Rn: 1},
			{Kind: "tag", ID: "t2", Category: "alpha", Rn: 2},
			{Kind: "latest", ID: "a4", Category: "beta", Rn: 1},
		}, rows)

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, feed.WithDryRun(&statements).Scan(&rows))
		require.Len(t, statements, 1)
		sql := statements[0].Query
		require.Contains(t, sql, ") AS q WHERE "+quoteIdent("q")+"."+quoteIdent("rn")+" = ",
			"the qualified member keeps its own wrap")
		qualified := strings.Index(sql, ") AS q WHERE ")
		pushed := strings.Index(sql, "ORDER BY "+quoteIdent("category")+" ASC,"+quoteIdent("id")+" ASC LIMIT ")
		require.Less(t, qualified, pushed, "the pushed order and cap sit outside the wrap, on the qualified rows")
		require.Less(t, pushed, strings.Index(sql, ") AS "+quoteIdent("b0")), "and inside the member")
		require.Equal(t, 3, strings.Count(statements[0].RenderedSQL, " LIMIT 4"), "both members and the union are capped")
	})
}

func TestUnionAllCount(t *testing.T) {
	defer cleanupUnionData()
	setupUnionData(t)
	ctx := context.Background()

	t.Run("AddsTheBranchCounts", func(t *testing.T) {
		total := 0
		require.NoError(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx), tagsBranch(ctx)).Count(&total))
		require.Equal(t, 9, total)

		require.NoError(t, database.UnionAll[feedRow](ctx,
			recordsBranch(ctx).Where(TestAggregateRecordCols.Category.Eq("alpha")), tagsBranch(ctx)).
			Count(&total))
		require.Equal(t, 6, total, "a branch's own filters narrow its count")
	})

	t.Run("IgnoresOrderingAndPaging", func(t *testing.T) {
		total := 0
		require.NoError(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx), tagsBranch(ctx)).
			OrderBy(TestAggregateRecordCols.ID.Desc()).Limit(2).Offset(3).
			Count(&total))
		require.Equal(t, 9, total)
	})

	t.Run("RendersTheCountsAdded", func(t *testing.T) {
		statements := make([]types.SQLStatement, 0)
		total := 0
		require.NoError(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx), tagsBranch(ctx)).
			OrderBy(TestAggregateRecordCols.ID.Desc()).Limit(2).
			WithDryRun(&statements).Count(&total))
		require.Len(t, statements, 1)
		member := func(table string) string {
			return "(SELECT COUNT(*) FROM (SELECT 1 AS " + quoteIdent("row_marker") + " FROM " + quoteIdent(table) +
				" WHERE " + quoteIdent(table) + "." + quoteIdent("deleted_at") + " IS NULL) AS "
		}
		require.Equal(t,
			"SELECT "+quoteIdent("n")+" FROM (SELECT "+member("test_aggregate_records")+quoteIdent("b0")+") + "+member("test_record_tags")+quoteIdent("b1")+") AS "+quoteIdent("n")+") AS "+quoteIdent("counts"),
			statements[0].Query,
			"the count adds scalar subqueries over what decides each branch's count and materializes no row")
		require.Empty(t, statements[0].Args, "the union's ordering and paging never reach the count")
	})

	t.Run("CountsTheGroupsOfAGroupedBranch", func(t *testing.T) {
		type row struct {
			Kind     string
			Category string
			Amount   int64
		}
		totals := database.Select[*TestAggregateRecord, row](ctx,
			types.Literal("total").As("kind"), TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum())
		each := database.Select[*TestAggregateRecord, row](ctx,
			types.Literal("row").As("kind"), TestAggregateRecordCols.Category, TestAggregateRecordCols.Amount)
		total := 0
		require.NoError(t, database.UnionAll[row](ctx, totals, each).Count(&total))
		require.Equal(t, 9, total, "three groups and six rows")

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, database.UnionAll[row](ctx, totals, each).WithDryRun(&statements).Count(&total))
		require.Len(t, statements, 1)
		require.Contains(t, statements[0].Query,
			"(SELECT COUNT(*) FROM (SELECT "+quoteIdent("category")+" AS "+quoteIdent("category")+" FROM "+quoteIdent("test_aggregate_records")+
				" WHERE "+quoteIdent("test_aggregate_records")+"."+quoteIdent("deleted_at")+" IS NULL GROUP BY "+quoteIdent("category")+") AS "+quoteIdent("b0")+")",
			"a grouped member counts its groups through its keys alone")
	})
}

func TestUnionAllJoinsTheContextTransaction(t *testing.T) {
	defer cleanupUnionData()
	setupUnionData(t)
	ctx := context.Background()

	// A row created inside the transaction is stacked by a union run on the
	// same context, and gone once the transaction rolls back: the union runs
	// on the transaction's connection, not beside it.
	rollback := errors.New("roll back")
	err := database.Transaction(ctx, func(ctx context.Context) error {
		if err := database.Database[*TestAggregateRecord](ctx).Create(&TestAggregateRecord{
			ID: "a7", Category: "delta", Status: "done", Amount: 700, Score: 7.5,
			OccurredAt: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		}); err != nil {
			return err
		}
		total := 0
		if err := database.UnionAll[feedRow](ctx, recordsBranch(ctx), tagsBranch(ctx)).Count(&total); err != nil {
			return err
		}
		require.Equal(t, 10, total)
		return rollback
	})
	require.ErrorIs(t, err, rollback)

	total := 0
	require.NoError(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx), tagsBranch(ctx)).Count(&total))
	require.Equal(t, 9, total)
}

func TestUnionAllBuildErrors(t *testing.T) {
	ctx := context.Background()
	rows := make([]feedRow, 0)
	total := 0

	t.Run("NoBranch", func(t *testing.T) {
		require.ErrorIs(t, database.UnionAll[feedRow](ctx).Scan(&rows), database.ErrUnionNoBranch)
		require.ErrorIs(t, database.UnionAll[feedRow](ctx).Count(&total), database.ErrUnionNoBranch)
	})

	t.Run("BranchCarryingOrderingOrPaging", func(t *testing.T) {
		for name, branch := range map[string]types.Selector[*TestAggregateRecord, feedRow]{
			"OrderBy": recordsBranch(ctx).OrderBy(TestAggregateRecordCols.ID.Asc()),
			"Limit":   recordsBranch(ctx).Limit(1),
			"Offset":  recordsBranch(ctx).Offset(1),
		} {
			t.Run(name, func(t *testing.T) {
				err := database.UnionAll[feedRow](ctx, tagsBranch(ctx), branch).Scan(&rows)
				require.ErrorIs(t, err, database.ErrNestedSelectOrdered)
				require.ErrorContains(t, err, "union branch 1")
			})
		}
	})

	t.Run("OrderByAColumnTheResultRowLacks", func(t *testing.T) {
		require.ErrorIs(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx)).
			OrderBy(TestAggregateRecordCols.Amount.Asc()).Scan(&rows), database.ErrUnionOrderNotSelected)
	})

	t.Run("OrderByATermNoBranchProjects", func(t *testing.T) {
		// The alias is a result column, but no branch projects this term
		// under it: the ordering would sort by something never selected.
		require.ErrorIs(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx)).
			OrderBy(TestAggregateRecordCols.Amount.Max().As("id").Desc()).Scan(&rows), database.ErrUnionOrderNotSelected)
	})

	t.Run("OffsetWithoutLimit", func(t *testing.T) {
		require.ErrorIs(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx)).Offset(1).Scan(&rows), database.ErrOffsetWithoutLimit)
	})

	t.Run("UnionAsBranch", func(t *testing.T) {
		// A union scans and counts like a branch, so the compiler lets it in;
		// only a select of this package can fill the role.
		inner := database.UnionAll[feedRow](ctx, recordsBranch(ctx))
		require.ErrorIs(t, database.UnionAll[feedRow](ctx, inner).Scan(&rows), database.ErrUnionBranch)
	})

	t.Run("BranchFailingItsOwnValidation", func(t *testing.T) {
		foreign := database.Select[*TestAggregateRecord, feedRow](ctx,
			types.Literal("record").As("kind"), TestRecordTagCols.ID, TestAggregateRecordCols.Category)
		err := database.UnionAll[feedRow](ctx, tagsBranch(ctx), foreign).Scan(&rows)
		require.ErrorIs(t, err, database.ErrColumnTable)
		require.ErrorContains(t, err, "union branch 1")
	})

	t.Run("NilDestinations", func(t *testing.T) {
		require.ErrorIs(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx)).Scan(nil), database.ErrNilDest)
		require.ErrorIs(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx)).Count(nil), database.ErrNilCount)
		require.ErrorIs(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx)).WithDryRun(nil).Scan(&rows), database.ErrNilSQLBuilder)
	})

	t.Run("PlainBranchOnItsOwnIsStillAPlainRead", func(t *testing.T) {
		// Stacking is what turns a projection of columns into a report; read
		// on its own the same select stays what List is for.
		require.ErrorIs(t, recordsBranch(ctx).Scan(&rows), database.ErrPlainSelect)
	})
}
