package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/tenant"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// The join tests read the seeded payments (see paymentSeed in
// fixture_test.go) beside the one seeded account (see accountSeed: acme
// alone, so bolt's payments meet no account), and the seeded records and
// tags (see aggregateSeed and tagSeed). Every expectation below is a literal
// a reader can check against those tables by hand.

// paymentAccount is the row a payment reads beside its account. AccountName
// is a pointer because a LEFT JOIN leaves it NULL for a payment without an
// account.
type paymentAccount struct {
	ID          string
	Account     string
	Amount      int64
	AccountName *string
}

func setupJoinData(t *testing.T) {
	t.Helper()
	cleanupFlowData()
	require.NoError(t, database.Database[*TestPayment](context.Background()).Create(paymentSeed()...))
	setupAccountData(t)
}

func cleanupJoinData() {
	cleanupFlowData()
	cleanupAccountData()
}

// joinedTable renders a quoted table.column the way the dialect under test
// quotes it.
func qualified(table, column string) string {
	return quoteIdent(table) + "." + quoteIdent(column)
}

func TestSelectJoinReadsTheJoinedColumns(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)
	ctx := context.Background()

	// Every tag beside its record's category and amount: the join pins the
	// record's primary key, so a tag matches one record.
	type taggedRecord struct {
		ID       string
		Label    string
		Category string
		Amount   int64
	}
	tagged := func(on types.Filter) types.Selector[*TestRecordTag, taggedRecord] {
		return database.Select[*TestRecordTag, taggedRecord](ctx,
			TestRecordTagCols.ID, TestRecordTagCols.Label, TestAggregateRecordCols.Category, TestAggregateRecordCols.Amount).
			Join(types.Join[*TestAggregateRecord](on)).
			OrderBy(TestRecordTagCols.ID.Asc())
	}
	want := []taggedRecord{
		{ID: "t1", Label: "vip", Category: "alpha", Amount: 100},
		{ID: "t2", Label: "vip", Category: "alpha", Amount: 300},
		{ID: "t3", Label: "bulk", Category: "beta", Amount: 400},
	}

	t.Run("ProjectsColumnsOfBothTables", func(t *testing.T) {
		rows := make([]taggedRecord, 0)
		require.NoError(t, tagged(TestAggregateRecordCols.ID.EqCol(TestRecordTagCols.RecordID)).Scan(&rows))
		require.Equal(t, want, rows)
	})

	t.Run("RendersEveryColumnQualifiedAndTheSoftDeleteInTheOn", func(t *testing.T) {
		statements := make([]types.SQLStatement, 0)
		rows := make([]taggedRecord, 0)
		require.NoError(t, tagged(TestAggregateRecordCols.ID.EqCol(TestRecordTagCols.RecordID)).WithDryRun(&statements).Scan(&rows))
		require.Len(t, statements, 1)
		tags, records := "test_record_tags", "test_aggregate_records"
		require.Equal(t,
			"SELECT "+qualified(tags, "id")+" AS "+quoteIdent("id")+", "+qualified(tags, "label")+" AS "+quoteIdent("label")+
				", "+qualified(records, "category")+" AS "+quoteIdent("category")+", "+qualified(records, "amount")+" AS "+quoteIdent("amount")+
				" FROM "+quoteIdent(tags)+
				" JOIN "+quoteIdent(records)+" ON "+qualified(records, "id")+" = "+qualified(tags, "record_id")+" AND "+qualified(records, "deleted_at")+" IS NULL"+
				" WHERE "+qualified(tags, "deleted_at")+" IS NULL ORDER BY "+quoteIdent("id")+" ASC",
			statements[0].Query,
			"a select that joins qualifies every column, and the joined model's soft-delete condition sits in the ON")
	})

	t.Run("EitherColumnMayBeWrittenFirst", func(t *testing.T) {
		// The predicate carries both tables, so the framework tells the
		// joined side from the queried side whichever is written first.
		rows := make([]taggedRecord, 0)
		reversed := tagged(TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID))
		require.NoError(t, reversed.Scan(&rows))
		require.Equal(t, want, rows)

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, reversed.WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query,
			" ON "+qualified("test_record_tags", "record_id")+" = "+qualified("test_aggregate_records", "id")+" AND ")
	})

	t.Run("OrdersByAJoinedColumn", func(t *testing.T) {
		rows := make([]taggedRecord, 0)
		require.NoError(t, database.Select[*TestRecordTag, taggedRecord](ctx,
			TestRecordTagCols.ID, TestRecordTagCols.Label, TestAggregateRecordCols.Category, TestAggregateRecordCols.Amount).
			Join(types.Join[*TestAggregateRecord](TestAggregateRecordCols.ID.EqCol(TestRecordTagCols.RecordID))).
			OrderBy(TestAggregateRecordCols.Amount.Desc()).
			Scan(&rows))
		require.Equal(t, []taggedRecord{want[2], want[1], want[0]}, rows)
	})
}

func TestSelectLeftJoinKeepsUnmatchedRows(t *testing.T) {
	defer cleanupJoinData()
	setupJoinData(t)
	ctx := context.Background()
	withAccount := func(source types.JoinSource) types.Selector[*TestPayment, paymentAccount] {
		return database.Select[*TestPayment, paymentAccount](ctx,
			TestPaymentCols.ID, TestPaymentCols.Account, TestPaymentCols.Amount, TestAccountCols.Name.As("account_name")).
			Join(source).
			OrderBy(TestPaymentCols.ID.Asc())
	}
	onCode := TestAccountCols.Code.EqCol(TestPaymentCols.Account)

	t.Run("LeftJoinLeavesTheMissingAccountNull", func(t *testing.T) {
		// The join pins the account's code, unique through Indexes: bolt has
		// no account row, so p3 keeps a NULL name instead of disappearing.
		rows := make([]paymentAccount, 0)
		require.NoError(t, withAccount(types.LeftJoin[*TestAccount](onCode)).Scan(&rows))
		require.Equal(t, []paymentAccount{
			{ID: "p1", Account: "acme", Amount: 100, AccountName: new("Acme Ltd")},
			{ID: "p2", Account: "acme", Amount: 200, AccountName: new("Acme Ltd")},
			{ID: "p3", Account: "bolt", Amount: 300, AccountName: nil},
			{ID: "p4", Account: "acme", Amount: 400, AccountName: new("Acme Ltd")},
		}, rows)

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, withAccount(types.LeftJoin[*TestAccount](onCode)).WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query,
			" FROM "+quoteIdent("test_payments")+" LEFT JOIN "+quoteIdent("test_accounts")+
				" ON "+qualified("test_accounts", "code")+" = "+qualified("test_payments", "account")+
				" AND "+qualified("test_accounts", "deleted_at")+" IS NULL WHERE ")
	})

	t.Run("JoinDropsThePaymentWithoutAnAccount", func(t *testing.T) {
		rows := make([]paymentAccount, 0)
		require.NoError(t, withAccount(types.Join[*TestAccount](onCode)).Scan(&rows))
		require.Equal(t, []string{"p1", "p2", "p4"}, []string{rows[0].ID, rows[1].ID, rows[2].ID})
		require.Len(t, rows, 3)
	})

	t.Run("CountsTheJoinedRows", func(t *testing.T) {
		inner, left := 0, 0
		require.NoError(t, withAccount(types.Join[*TestAccount](onCode)).Count(&inner))
		require.NoError(t, withAccount(types.LeftJoin[*TestAccount](onCode)).Count(&left))
		require.Equal(t, 3, inner)
		require.Equal(t, 4, left)
	})

	t.Run("LeftJoinedFieldMustHoldNull", func(t *testing.T) {
		type plain struct {
			ID          string
			AccountName string
		}
		rows := make([]plain, 0)
		err := database.Select[*TestPayment, plain](ctx, TestPaymentCols.ID, TestAccountCols.Name.As("account_name")).
			Join(types.LeftJoin[*TestAccount](onCode)).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrNullableResultField)
		require.ErrorContains(t, err, "LEFT JOIN")
	})
}

func TestSelectJoinPredicatesAndFilters(t *testing.T) {
	defer cleanupJoinData()
	setupJoinData(t)
	ctx := context.Background()
	onCode := TestAccountCols.Code.EqCol(TestPaymentCols.Account)

	t.Run("OnTakesConditionsBesideTheKey", func(t *testing.T) {
		ids := func(t *testing.T, tier string) []string {
			t.Helper()
			rows := make([]paymentAccount, 0)
			require.NoError(t, database.Select[*TestPayment, paymentAccount](ctx,
				TestPaymentCols.ID, TestPaymentCols.Account, TestPaymentCols.Amount, TestAccountCols.Name.As("account_name")).
				Join(types.Join[*TestAccount](onCode, TestAccountCols.Tier.Eq(tier))).
				OrderBy(TestPaymentCols.ID.Asc()).
				Scan(&rows))
			collected := make([]string, 0, len(rows))
			for _, r := range rows {
				collected = append(collected, r.ID)
			}
			return collected
		}
		require.Equal(t, []string{"p1", "p2", "p4"}, ids(t, "gold"))
		require.Empty(t, ids(t, "silver"), "a condition in the ON narrows which account can match")

		statements := make([]types.SQLStatement, 0)
		rows := make([]paymentAccount, 0)
		require.NoError(t, database.Select[*TestPayment, paymentAccount](ctx,
			TestPaymentCols.ID, TestPaymentCols.Account, TestPaymentCols.Amount, TestAccountCols.Name.As("account_name")).
			Join(types.Join[*TestAccount](onCode, TestAccountCols.Tier.Eq("gold"))).
			WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query,
			" ON ("+qualified("test_accounts", "code")+" = "+qualified("test_payments", "account")+" AND "+qualified("test_accounts", "tier")+" = ")
	})

	t.Run("WhereReadsBothTablesQualified", func(t *testing.T) {
		rows := make([]paymentAccount, 0)
		sel := database.Select[*TestPayment, paymentAccount](ctx,
			TestPaymentCols.ID, TestPaymentCols.Account, TestPaymentCols.Amount, TestAccountCols.Name.As("account_name")).
			Join(types.LeftJoin[*TestAccount](onCode)).
			Where(TestPaymentCols.Amount.Gte(200), TestAccountCols.Tier.Eq("gold")).
			OrderBy(TestPaymentCols.ID.Asc())
		require.NoError(t, sel.Scan(&rows))
		require.Equal(t, []paymentAccount{
			{ID: "p2", Account: "acme", Amount: 200, AccountName: new("Acme Ltd")},
			{ID: "p4", Account: "acme", Amount: 400, AccountName: new("Acme Ltd")},
		}, rows)

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, sel.WithDryRun(&statements).Scan(&rows))
		sql := statements[0].Query
		require.Contains(t, sql, " WHERE ("+qualified("test_payments", "amount")+" >= ")
		require.Contains(t, sql, " AND "+qualified("test_accounts", "tier")+" = ")
		require.Contains(t, sql, ") AND "+qualified("test_payments", "deleted_at")+" IS NULL ORDER BY "+quoteIdent("id")+" ASC")
	})

	t.Run("ConditionalMeasureReadsTheJoinedTable", func(t *testing.T) {
		type totals struct {
			Gold int64
			All  int64
		}
		got := totals{}
		require.NoError(t, database.Select[*TestPayment, totals](ctx,
			TestPaymentCols.Amount.Sum().Where(TestAccountCols.Tier.Eq("gold")).As("gold"),
			TestPaymentCols.Amount.Sum().As("all")).
			Join(types.LeftJoin[*TestAccount](onCode)).
			ScanOne(&got))
		require.Equal(t, totals{Gold: 700, All: 1000}, got)
	})
}

func TestSelectJoinGroupedAndWindowed(t *testing.T) {
	defer cleanupJoinData()
	setupJoinData(t)
	ctx := context.Background()
	onCode := TestAccountCols.Code.EqCol(TestPaymentCols.Account)

	t.Run("GroupsByTheQueriedModelWithMaxOverTheJoined", func(t *testing.T) {
		type accountTotal struct {
			Account string
			Name    *string
			Amount  int64
		}
		rows := make([]accountTotal, 0)
		sel := database.Select[*TestPayment, accountTotal](ctx,
			TestPaymentCols.Account.Group(), TestAccountCols.Name.Max().As("name"), TestPaymentCols.Amount.Sum()).
			Join(types.LeftJoin[*TestAccount](onCode)).
			OrderBy(TestPaymentCols.Account.Group().Asc())
		require.NoError(t, sel.Scan(&rows))
		require.Equal(t, []accountTotal{
			{Account: "acme", Name: new("Acme Ltd"), Amount: 700},
			{Account: "bolt", Name: nil, Amount: 300},
		}, rows)

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, sel.WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query, "MAX("+qualified("test_accounts", "name")+") AS "+quoteIdent("name"))
		require.Contains(t, statements[0].Query, " GROUP BY "+qualified("test_payments", "account")+" ORDER BY "+quoteIdent("account")+" ASC")
	})

	t.Run("GroupsByAJoinedColumn", func(t *testing.T) {
		type tierTotal struct {
			Tier   *string
			Amount int64
		}
		rows := make([]tierTotal, 0)
		require.NoError(t, database.Select[*TestPayment, tierTotal](ctx,
			TestAccountCols.Tier.Group(), TestPaymentCols.Amount.Sum()).
			Join(types.LeftJoin[*TestAccount](onCode)).
			OrderBy(TestPaymentCols.Amount.Sum().Desc()).
			Scan(&rows))
		require.Equal(t, []tierTotal{
			{Tier: new("gold"), Amount: 700},
			{Tier: nil, Amount: 300},
		}, rows, "the unmatched payments group under the NULL tier")
	})

	t.Run("CountsDistinctJoinedValues", func(t *testing.T) {
		type tiers struct {
			Tiers int64
		}
		got := tiers{}
		require.NoError(t, database.Select[*TestPayment, tiers](ctx, TestAccountCols.Tier.CountDistinct().As("tiers")).
			Join(types.LeftJoin[*TestAccount](onCode)).
			ScanOne(&got))
		require.Equal(t, int64(1), got.Tiers)
	})

	t.Run("PartitionsAWindowByAJoinedColumn", func(t *testing.T) {
		type latest struct {
			ID   string
			Tier *string
			Rn   int64
		}
		rn := types.RowNumber().
			Over(types.PartitionBy(TestAccountCols.Tier).OrderBy(TestPaymentCols.PaidAt.Desc())).
			As("rn")
		sel := database.Select[*TestPayment, latest](ctx, TestPaymentCols.ID, TestAccountCols.Tier, rn).
			Join(types.LeftJoin[*TestAccount](onCode)).
			Qualify(rn.Eq(1)).
			OrderBy(TestPaymentCols.ID.Asc())
		rows := make([]latest, 0)
		require.NoError(t, sel.Scan(&rows))
		require.Equal(t, []latest{
			{ID: "p3", Tier: nil, Rn: 1},
			{ID: "p4", Tier: new("gold"), Rn: 1},
		}, rows, "the latest payment of the gold tier and of the payments without an account")

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, sel.WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query,
			"ROW_NUMBER() OVER (PARTITION BY "+qualified("test_accounts", "tier")+
				" ORDER BY "+qualified("test_payments", "paid_at")+" DESC, "+qualified("test_payments", "id")+" ASC) AS "+quoteIdent("rn"),
			"the window's keys and tie breaker are qualified like every other column")
	})

	t.Run("PartitionsGroupsByAJoinedKey", func(t *testing.T) {
		// Both group keys are named id; the partition names the account's,
		// and every payment of an account shares the account's total. A
		// partition by the payment's own id would answer each payment's own
		// amount instead.
		type share struct {
			ID        string
			AccountID *string
			Amount    int64
			Share     int64
		}
		rows := make([]share, 0)
		sel := database.Select[*TestPayment, share](ctx,
			TestPaymentCols.ID.Group(), TestAccountCols.ID.Group().As("account_id"),
			TestPaymentCols.Amount.Sum(), TestPaymentCols.Amount.Sum().Over(types.PartitionBy(TestAccountCols.ID)).As("share")).
			Join(types.LeftJoin[*TestAccount](onCode)).
			OrderBy(TestPaymentCols.ID.Group().Asc())
		require.NoError(t, sel.Scan(&rows))
		require.Equal(t, []share{
			{ID: "p1", AccountID: new("acc1"), Amount: 100, Share: 700},
			{ID: "p2", AccountID: new("acc1"), Amount: 200, Share: 700},
			{ID: "p3", AccountID: nil, Amount: 300, Share: 300},
			{ID: "p4", AccountID: new("acc1"), Amount: 400, Share: 700},
		}, rows)

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, sel.WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query, "OVER (PARTITION BY "+qualified("test_accounts", "id")+")")
	})

	t.Run("PartitionByAJoinedColumnThatIsNotAKey", func(t *testing.T) {
		// The payment's id is a group key, the account's is not: the name
		// alone matches, the table refuses.
		type wrong struct {
			ID    string
			Share int64
		}
		err := database.Select[*TestPayment, wrong](ctx, TestPaymentCols.ID.Group(),
			TestPaymentCols.Amount.Sum().Over(types.PartitionBy(TestAccountCols.ID)).As("share")).
			Join(types.LeftJoin[*TestAccount](onCode)).
			Scan(&[]wrong{})
		require.ErrorIs(t, err, database.ErrWindowTermNotSelected)
	})
}

func TestSelectJoinRowLevelReads(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)
	ctx := context.Background()
	onRecord := TestAggregateRecordCols.ID.EqCol(TestRecordTagCols.RecordID)

	t.Run("WindowedSumOverALeftJoinedColumnIsNeverNull", func(t *testing.T) {
		// The renderer coalesces SUM, so the running total of a LEFT JOIN
		// column reads into a plain field: an unmatched row adds zero.
		type running struct {
			ID    string
			Total int64
		}
		// t4 points at no record: its LEFT JOIN side is NULL and adds zero.
		require.NoError(t, database.Database[*TestRecordTag](ctx).Create(&TestRecordTag{ID: "t4", RecordID: "zz", Label: "loose", Category: "none"}))
		rows := make([]running, 0)
		require.NoError(t, database.Select[*TestRecordTag, running](ctx, TestRecordTagCols.ID,
			TestAggregateRecordCols.Amount.Sum().Over(types.OrderBy(TestRecordTagCols.ID.Asc())).As("total")).
			Join(types.LeftJoin[*TestAggregateRecord](onRecord)).
			OrderBy(TestRecordTagCols.ID.Asc()).
			Scan(&rows))
		require.Equal(t, []running{{ID: "t1", Total: 100}, {ID: "t2", Total: 400}, {ID: "t3", Total: 800}, {ID: "t4", Total: 800}}, rows)
	})

	t.Run("PartitionsByTheDerivedTerm", func(t *testing.T) {
		// A joined select's term is a column of the derived table in a
		// row-level projection too: the window partitions by it, as it does
		// in a grouped one.
		type numbered struct {
			ID   string
			Tags *int64
			Rn   int64
		}
		tags, counts := tagCounts(ctx)
		statements := make([]types.SQLStatement, 0)
		rows := make([]numbered, 0)
		sel := database.Select[*TestAggregateRecord, numbered](ctx, TestAggregateRecordCols.ID, tags,
			types.RowNumber().Over(types.PartitionBy(tags).OrderBy(TestAggregateRecordCols.ID.Asc())).As("rn")).
			Join(types.LeftJoinSelect(counts, TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID))).
			OrderBy(TestAggregateRecordCols.ID.Asc())
		require.NoError(t, sel.WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query, "OVER (PARTITION BY "+qualified("j0", "tags")+" ORDER BY ")
		require.NoError(t, sel.Scan(&rows))
		// a1, a3 and a4 carry one tag each and share a partition; the untagged
		// rows share the NULL one.
		require.Equal(t, []numbered{
			{ID: "a1", Tags: new(int64(1)), Rn: 1},
			{ID: "a2", Rn: 1},
			{ID: "a3", Tags: new(int64(1)), Rn: 2},
			{ID: "a4", Tags: new(int64(1)), Rn: 3},
			{ID: "a5", Rn: 2},
			{ID: "a6", Rn: 3},
		}, rows)
	})

	t.Run("PlainNameOrdersByTheQueriedModelsColumn", func(t *testing.T) {
		// Both tables project a category; a plain name orders by the queried
		// model's, whichever of the two is projected first.
		type tagged struct {
			ID             string
			RecordCategory string
			Category       string
		}
		rows := make([]tagged, 0)
		sel := database.Select[*TestRecordTag, tagged](ctx,
			TestAggregateRecordCols.Category.As("record_category"), TestRecordTagCols.ID, TestRecordTagCols.Category).
			Join(types.Join[*TestAggregateRecord](onRecord)).
			OrderBy(types.Desc("category"), types.Asc("id"))
		require.NoError(t, sel.Scan(&rows))
		require.Equal(t, []tagged{
			{ID: "t3", RecordCategory: "beta", Category: "beta"},
			{ID: "t1", RecordCategory: "alpha", Category: "alpha"},
			{ID: "t2", RecordCategory: "alpha", Category: "alpha"},
		}, rows)

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, sel.WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query, " ORDER BY "+quoteIdent("category")+" DESC,"+quoteIdent("id")+" ASC")
	})

	t.Run("TieBreakerIsTheQueriedModelsKey", func(t *testing.T) {
		// The joined select projects its count under the primary key's
		// name; the window's tie breaker still reads the queried model's
		// key, not the derived column that shares the name.
		perRecord := TestRecordTagCols.ID.Count().As("id")
		counts := database.Select[*TestRecordTag, struct {
			RecordID string
			ID       int64
		}](ctx, TestRecordTagCols.RecordID.Group(), perRecord)
		type numbered struct {
			Category string
			ID       *int64
			Rn       int64
		}
		statements := make([]types.SQLStatement, 0)
		rows := make([]numbered, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, numbered](ctx, TestAggregateRecordCols.Category, perRecord,
			types.RowNumber().Over(types.OrderBy(TestAggregateRecordCols.Category.Asc())).As("rn")).
			Join(types.LeftJoinSelect(counts, TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID))).
			WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query,
			"ROW_NUMBER() OVER (ORDER BY "+qualified("test_aggregate_records", "category")+" ASC, "+qualified("test_aggregate_records", "id")+" ASC)")
	})

	t.Run("JSONContainsIsQualified", func(t *testing.T) {
		// The JSON predicate quotes its own operand, so it is handed the
		// column under its table; bare, two tables with the column name
		// would make the statement ambiguous.
		userID := types.NewColumn[*TestUser, string]("id")
		userAddr := types.NewColumn[*TestUser, datatypes.JSONSlice[string]]("addr")
		type named struct {
			ID   string
			Name string
		}
		statements := make([]types.SQLStatement, 0)
		rows := make([]named, 0)
		require.NoError(t, database.Select[*TestRecordTag, named](ctx, TestRecordTagCols.ID, colName.As("name")).
			Join(types.Join[*TestUser](userID.EqCol(TestRecordTagCols.RecordID))).
			Where(userAddr.JSONContains("home")).
			WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query, qualified("test_users", "addr"))
		require.NotContains(t, statements[0].Query, "("+quoteIdent("addr")+")")
	})
}

func TestSelectJoinKeepsTheJoinedModelsTenant(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, database.DB().AutoMigrate(&TestTenantSoftDeleteItem{}))
	cleanup := func() {
		_ = database.DB().Exec("DELETE FROM test_tenant_soft_delete_items").Error
		cleanupTagData()
	}
	defer cleanup()
	cleanup()

	// One item per tenant, each tagged; a read in tenant-a joins the tag
	// of tenant-b's item to nothing, the way a List in tenant-a hides that
	// item.
	mine := &TestTenantSoftDeleteItem{Name: "mine"}
	theirs := &TestTenantSoftDeleteItem{Name: "theirs"}
	require.NoError(t, database.Database[*TestTenantSoftDeleteItem](tenant.In(ctx, "tenant-a")).Create(mine))
	require.NoError(t, database.Database[*TestTenantSoftDeleteItem](tenant.In(ctx, "tenant-b")).Create(theirs))
	require.NoError(t, database.Database[*TestRecordTag](ctx).Create(
		&TestRecordTag{ID: "tg1", RecordID: mine.ID, Label: "a"},
		&TestRecordTag{ID: "tg2", RecordID: theirs.ID, Label: "b"},
	))
	itemID := types.NewColumn[*TestTenantSoftDeleteItem, string]("id")
	itemName := types.NewColumn[*TestTenantSoftDeleteItem, string]("name")
	type taggedItem struct {
		ID   string
		Name *string
	}
	read := func(ctx context.Context) ([]taggedItem, []types.SQLStatement) {
		t.Helper()
		sel := database.Select[*TestRecordTag, taggedItem](ctx, TestRecordTagCols.ID, itemName.As("name")).
			Join(types.LeftJoin[*TestTenantSoftDeleteItem](itemID.EqCol(TestRecordTagCols.RecordID))).
			OrderBy(TestRecordTagCols.ID.Asc())
		rows := make([]taggedItem, 0)
		require.NoError(t, sel.Scan(&rows))
		statements := make([]types.SQLStatement, 0)
		require.NoError(t, sel.WithDryRun(&statements).Scan(&rows))
		return rows, statements
	}

	rows, statements := read(tenant.In(ctx, "tenant-a"))
	require.Equal(t, []taggedItem{{ID: "tg1", Name: new("mine")}, {ID: "tg2", Name: nil}}, rows)
	require.Contains(t, statements[0].Query,
		" AND "+qualified("test_tenant_soft_delete_items", "deleted_at")+" IS NULL AND "+qualified("test_tenant_soft_delete_items", "tenant_id")+" = ",
		"the tenant condition sits in the ON beside the soft delete")
	require.Equal(t, []any{"tenant-a"}, statements[0].Args)

	rows, statements = read(tenant.Across(ctx))
	require.Equal(t, []taggedItem{{ID: "tg1", Name: new("mine")}, {ID: "tg2", Name: new("theirs")}}, rows, "a cross-tenant context reads every tenant's rows")
	require.NotContains(t, statements[0].Query, "tenant_id")
}

func TestSelectJoinInsideAUnionBranch(t *testing.T) {
	defer cleanupJoinData()
	setupJoinData(t)
	ctx := context.Background()

	// A joining select is an ordinary branch: both branches read the account
	// beside their rows, and the union stacks them.
	type flowAccount struct {
		Kind        string
		ID          string
		AccountName *string
	}
	payments := database.Select[*TestPayment, flowAccount](ctx,
		types.Literal("payment").As("kind"), TestPaymentCols.ID, TestAccountCols.Name.As("account_name")).
		Join(types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestPaymentCols.Account)))
	refunds := database.Select[*TestRefund, flowAccount](ctx,
		types.Literal("refund").As("kind"), TestRefundCols.ID, TestAccountCols.Name.As("account_name")).
		Join(types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestRefundCols.Account)))
	require.NoError(t, database.Database[*TestRefund](ctx).Create(refundSeed()...))

	rows := make([]flowAccount, 0)
	require.NoError(t, database.UnionAll[flowAccount](ctx, payments, refunds).
		OrderBy(TestPaymentCols.ID.Asc()).
		Scan(&rows))
	require.Equal(t, []flowAccount{
		{Kind: "payment", ID: "p1", AccountName: new("Acme Ltd")},
		{Kind: "payment", ID: "p2", AccountName: new("Acme Ltd")},
		{Kind: "payment", ID: "p3", AccountName: nil},
		{Kind: "payment", ID: "p4", AccountName: new("Acme Ltd")},
		{Kind: "refund", ID: "r1", AccountName: new("Acme Ltd")},
		{Kind: "refund", ID: "r2", AccountName: nil},
	}, rows)
}

func TestSelectJoinBuildErrors(t *testing.T) {
	ctx := context.Background()
	rows := make([]paymentAccount, 0)
	onCode := TestAccountCols.Code.EqCol(TestPaymentCols.Account)
	withAccount := func(source types.JoinSource) types.Selector[*TestPayment, paymentAccount] {
		return database.Select[*TestPayment, paymentAccount](ctx,
			TestPaymentCols.ID, TestPaymentCols.Account, TestPaymentCols.Amount, TestAccountCols.Name.As("account_name")).
			Join(source)
	}

	t.Run("OneToManyIsNotUnique", func(t *testing.T) {
		// A record has many tags: record_id pins no unique key of the tags,
		// so a record could match several and SUM would multiply.
		type recordTag struct {
			ID    string
			Label string
		}
		tagRows := make([]recordTag, 0)
		err := database.Select[*TestAggregateRecord, recordTag](ctx, TestAggregateRecordCols.ID, TestRecordTagCols.Label).
			Join(types.Join[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID))).
			Scan(&tagRows)
		require.ErrorIs(t, err, database.ErrJoinNotUnique)
		require.ErrorContains(t, err, "record_id")
	})

	t.Run("NoPredicateTiesTheTables", func(t *testing.T) {
		require.ErrorIs(t, withAccount(types.Join[*TestAccount](TestAccountCols.Tier.Eq("gold"))).Scan(&rows), database.ErrJoinNoCorrelation)
	})

	t.Run("PlainNamesCannotTie", func(t *testing.T) {
		// The string form of EqCol carries no tables, so it cannot say which
		// side is the joined one and proves nothing.
		err := withAccount(types.Join[*TestAccount](types.FilterEqCol("code", "account"))).Scan(&rows)
		require.ErrorIs(t, err, database.ErrJoinNoCorrelation)
		require.ErrorContains(t, err, "names a column without a table")
	})

	t.Run("TiedToATableNoSourceReads", func(t *testing.T) {
		// An ON tied to a table the query reads through no source declared
		// before it names that table: the join is declared too early, or
		// the table is not joined at all.
		err := withAccount(types.Join[*TestAccount](TestAccountCols.Code.EqCol(TestRecordTagCols.Category))).Scan(&rows)
		require.ErrorIs(t, err, database.ErrJoinNoCorrelation)
		require.ErrorContains(t, err, `ties it to "test_record_tags", which no source declared before it reads`)
	})

	t.Run("KeyInsideAnOrGroupProvesNothing", func(t *testing.T) {
		require.ErrorIs(t, withAccount(types.Join[*TestAccount](types.FilterOr(onCode, TestAccountCols.Code.Eq("acme")))).Scan(&rows), database.ErrJoinNoCorrelation)
	})

	t.Run("ValueEqualityPinsAKeyToo", func(t *testing.T) {
		// Pinning the unique code to a value proves uniqueness without a
		// column pair, but nothing ties the account to the payment: the
		// join must still name a column of the query.
		require.ErrorIs(t, withAccount(types.Join[*TestAccount](TestAccountCols.Code.Eq("acme"))).Scan(&rows), database.ErrJoinNoCorrelation)
	})

	t.Run("ModelJoinedTwice", func(t *testing.T) {
		err := database.Select[*TestPayment, paymentAccount](ctx,
			TestPaymentCols.ID, TestPaymentCols.Account, TestPaymentCols.Amount, TestAccountCols.Name.As("account_name")).
			Join(types.Join[*TestAccount](onCode), types.Join[*TestAccount](onCode)).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrJoinDuplicateTable)
	})

	t.Run("ModelJoiningItself", func(t *testing.T) {
		require.ErrorIs(t, withAccount(types.Join[*TestPayment](TestPaymentCols.ID.EqCol(TestPaymentCols.Account))).Scan(&rows), database.ErrJoinDuplicateTable)
	})

	t.Run("JoinedSelectOverTheQueriedTable", func(t *testing.T) {
		// A select over the payments is addressed through the payment
		// columns, the same references the query reads its own table by; the
		// error names the source so the reader is not sent looking for a
		// model join.
		type accountTotal struct {
			Account string
			Total   int64
		}
		total := TestPaymentCols.Amount.Sum().As("total")
		perAccount := database.Select[*TestPayment, accountTotal](ctx, TestPaymentCols.Account.Group(), total)
		type paymentShare struct {
			ID    string
			Total *int64
		}
		err := database.Select[*TestPayment, paymentShare](ctx, TestPaymentCols.ID, total).
			Join(types.LeftJoinSelect(perAccount, TestPaymentCols.Account.EqCol(TestPaymentCols.Account))).
			Scan(&[]paymentShare{})
		require.ErrorIs(t, err, database.ErrJoinDuplicateTable)
		require.ErrorContains(t, err, "joined select over")
	})

	t.Run("SumOverAJoinedColumnInAGroupedSelect", func(t *testing.T) {
		type wrong struct {
			Account string
			Names   int64
		}
		wrongRows := make([]wrong, 0)
		err := database.Select[*TestPayment, wrong](ctx, TestPaymentCols.Account.Group(), TestAccountCols.Name.Count().As("names")).
			Join(types.LeftJoin[*TestAccount](onCode)).
			Scan(&wrongRows)
		require.ErrorIs(t, err, database.ErrJoinMeasure)
	})

	t.Run("ColumnOfATableNotJoined", func(t *testing.T) {
		type total struct {
			Amount int64
			Name   *string
		}
		got := total{}
		err := database.Select[*TestPayment, total](ctx, TestPaymentCols.Amount.Sum(), TestAccountCols.Name.Max().As("name")).
			ScanOne(&got)
		require.ErrorIs(t, err, database.ErrColumnTable)
	})

	t.Run("EqColOfATableNotJoined", func(t *testing.T) {
		// The other side of an EqCol names a table the query does not read:
		// refused under both sentinels, the way a filter naming that table is.
		err := withAccount(types.LeftJoin[*TestAccount](onCode)).
			Where(TestPaymentCols.Account.EqCol(TestRecordTagCols.Label)).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrColumnTable)
		require.ErrorIs(t, err, database.ErrUnusableFilter)
	})

	t.Run("FilterOfATableNotJoined", func(t *testing.T) {
		type total struct {
			Amount int64
		}
		got := total{}
		err := database.Select[*TestPayment, total](ctx, TestPaymentCols.Amount.Sum()).
			Where(TestAccountCols.Tier.Eq("gold")).
			ScanOne(&got)
		require.ErrorIs(t, err, database.ErrUnusableFilter)
		require.ErrorContains(t, err, "does not read")
	})

	t.Run("OrderByAColumnOfATableNotJoined", func(t *testing.T) {
		err := withAccount(types.LeftJoin[*TestAccount](onCode)).OrderBy(TestRefundCols.ID.Asc()).Scan(&rows)
		require.ErrorIs(t, err, database.ErrOrderTermNotSelected)
	})
}

// tagsPerRecord is the row of the grouped select the joined-select tests
// read: the tags of every record counted, one row per record.
type tagsPerRecord struct {
	RecordID string
	Tags     int64
}

// tagCounts is that select, with the term the query reads back from it.
func tagCounts(ctx context.Context) (types.Term, types.Selector[*TestRecordTag, tagsPerRecord]) {
	tags := TestRecordTagCols.ID.Count().As("tags")
	return tags, database.Select[*TestRecordTag, tagsPerRecord](ctx, TestRecordTagCols.RecordID.Group(), tags)
}

func TestSelectJoinSelectReadsTheDerivedTerms(t *testing.T) {
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)
	ctx := context.Background()

	// Every record with the number of its tags: the many side is grouped by
	// the record first, so a record matches one group at most, and the
	// query reads the count back as a column of the derived table.
	type recordTags struct {
		ID       string
		Category string
		Tags     *int64
	}
	withTags := func(source func(types.Selector[*TestRecordTag, tagsPerRecord], types.Filter) types.JoinSource, on func() types.Filter) (types.Term, types.Selector[*TestAggregateRecord, recordTags]) {
		tags, counts := tagCounts(ctx)
		return tags, database.Select[*TestAggregateRecord, recordTags](ctx,
			TestAggregateRecordCols.ID, TestAggregateRecordCols.Category, tags).
			Join(source(counts, on())).
			OrderBy(TestAggregateRecordCols.ID.Asc())
	}
	left := func(sub types.Selector[*TestRecordTag, tagsPerRecord], on types.Filter) types.JoinSource {
		return types.LeftJoinSelect(sub, on)
	}
	onRecord := func() types.Filter { return TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID) }

	t.Run("LeftJoinSelectLeavesRecordsWithoutTagsNull", func(t *testing.T) {
		_, sel := withTags(left, onRecord)
		rows := make([]recordTags, 0)
		require.NoError(t, sel.Scan(&rows))
		require.Equal(t, []recordTags{
			{ID: "a1", Category: "alpha", Tags: new(int64(1))},
			{ID: "a2", Category: "alpha", Tags: nil},
			{ID: "a3", Category: "alpha", Tags: new(int64(1))},
			{ID: "a4", Category: "beta", Tags: new(int64(1))},
			{ID: "a5", Category: "beta", Tags: nil},
			{ID: "a6", Category: "gamma", Tags: nil},
		}, rows)
	})

	t.Run("RendersTheDerivedTableAndReadsItsColumn", func(t *testing.T) {
		_, sel := withTags(left, onRecord)
		statements := make([]types.SQLStatement, 0)
		rows := make([]recordTags, 0)
		require.NoError(t, sel.WithDryRun(&statements).Scan(&rows))
		require.Len(t, statements, 1)
		records, tags := "test_aggregate_records", "test_record_tags"
		require.Equal(t,
			"SELECT "+qualified(records, "id")+" AS "+quoteIdent("id")+", "+qualified(records, "category")+" AS "+quoteIdent("category")+
				", "+qualified("j0", "tags")+" AS "+quoteIdent("tags")+
				" FROM "+quoteIdent(records)+
				" LEFT JOIN (SELECT "+quoteIdent("record_id")+" AS "+quoteIdent("record_id")+", COUNT("+quoteIdent("id")+") AS "+quoteIdent("tags")+
				" FROM "+quoteIdent(tags)+" WHERE "+qualified(tags, "deleted_at")+" IS NULL GROUP BY "+quoteIdent("record_id")+") AS "+quoteIdent("j0")+
				" ON "+qualified("j0", "record_id")+" = "+qualified(records, "id")+
				" WHERE "+qualified(records, "deleted_at")+" IS NULL ORDER BY "+quoteIdent("id")+" ASC",
			statements[0].Query,
			"the grouped select is a derived table, its key is read under the key's alias, and the term the query passed again reads as its column")
	})

	t.Run("JoinSelectKeepsOnlyTaggedRecords", func(t *testing.T) {
		// Inner: the count can no longer be NULL, so the field needs no pointer.
		type tagged struct {
			ID   string
			Tags int64
		}
		tags, counts := tagCounts(ctx)
		rows := make([]tagged, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, tagged](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.JoinSelect(counts, onRecord())).
			OrderBy(TestAggregateRecordCols.ID.Asc()).
			Scan(&rows))
		require.Equal(t, []tagged{{ID: "a1", Tags: 1}, {ID: "a3", Tags: 1}, {ID: "a4", Tags: 1}}, rows)

		inner, leftCount := 0, 0
		require.NoError(t, database.Select[*TestAggregateRecord, tagged](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.JoinSelect(counts, onRecord())).Count(&inner))
		_, sel := withTags(left, onRecord)
		require.NoError(t, sel.Count(&leftCount))
		require.Equal(t, 3, inner)
		require.Equal(t, 6, leftCount)
	})

	t.Run("EitherColumnMayBeWrittenFirst", func(t *testing.T) {
		_, sel := withTags(left, func() types.Filter { return TestAggregateRecordCols.ID.EqCol(TestRecordTagCols.RecordID) })
		rows := make([]recordTags, 0)
		require.NoError(t, sel.Scan(&rows))
		require.Len(t, rows, 6)
		statements := make([]types.SQLStatement, 0)
		require.NoError(t, sel.WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query, ") AS "+quoteIdent("j0")+" ON "+qualified("test_aggregate_records", "id")+" = "+qualified("j0", "record_id")+" WHERE ")
	})

	t.Run("NestedJoinedSelectIsTiedByItsOwnKeys", func(t *testing.T) {
		// The tags count their notes through a select of their own; a query
		// joining that select ties it on the tag id alone: the notes are a
		// column of the derived table, not a key a column of the query equals.
		noteID := types.NewColumn[*TestTagNote, string]("id")
		noteTag := types.NewColumn[*TestTagNote, string]("tag_id")
		notes := noteID.Count().As("notes")
		perTag := database.Select[*TestTagNote, struct {
			TagID string
			Notes int64
		}](ctx, noteTag.Group(), notes)
		perRecordTag := database.Select[*TestRecordTag, struct {
			ID    string
			Notes *int64
		}](ctx, TestRecordTagCols.ID.Group(), notes).
			Join(types.LeftJoinSelect(perTag, noteTag.EqCol(TestRecordTagCols.ID)))
		type noted struct {
			ID    string
			Notes *int64
		}
		statements := make([]types.SQLStatement, 0)
		rows := make([]noted, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, noted](ctx, TestAggregateRecordCols.ID, notes).
			Join(types.LeftJoinSelect(perRecordTag, TestRecordTagCols.ID.EqCol(TestAggregateRecordCols.ID))).
			WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query, ") AS "+quoteIdent("j0")+" ON "+qualified("j0", "id")+" = "+qualified("test_aggregate_records", "id"))
		require.Contains(t, statements[0].Query, qualified("j0", "notes")+" AS "+quoteIdent("notes"))
	})

	t.Run("PartitionsByTheDerivedTerm", func(t *testing.T) {
		// A grouped query groups by the term it reads from the select, so a
		// window may partition by it: every record with the same tag count
		// shares the partition.
		type share struct {
			ID    string
			Tags  *int64
			Share int64
		}
		tags, counts := tagCounts(ctx)
		statements := make([]types.SQLStatement, 0)
		rows := make([]share, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, share](ctx,
			TestAggregateRecordCols.ID.Group(), tags,
			TestAggregateRecordCols.Amount.Sum().Over(types.PartitionBy(tags)).As("share")).
			Join(types.LeftJoinSelect(counts, onRecord())).
			WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query, "OVER (PARTITION BY "+qualified("j0", "tags")+")")
	})

	t.Run("JoinsOnAColumnTheQueryRepeats", func(t *testing.T) {
		// The derived table is unique on its key; the query's side need not
		// be: every alpha record reads alpha's tag count.
		type categoryTags struct {
			ID   string
			Tags int64
		}
		tags := TestRecordTagCols.ID.Count().As("tags")
		perCategory := database.Select[*TestRecordTag, struct {
			Category string
			Tags     int64
		}](ctx, TestRecordTagCols.Category.Group(), tags)
		rows := make([]categoryTags, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, categoryTags](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.JoinSelect(perCategory, TestRecordTagCols.Category.EqCol(TestAggregateRecordCols.Category))).
			OrderBy(tags.Desc(), TestAggregateRecordCols.ID.Asc()).
			Scan(&rows))
		require.Equal(t, []categoryTags{
			{ID: "a1", Tags: 2},
			{ID: "a2", Tags: 2},
			{ID: "a3", Tags: 2},
			{ID: "a4", Tags: 1},
			{ID: "a5", Tags: 1},
		}, rows, "ordering by the derived term sorts the query's rows by the count they read")
	})

	t.Run("FiltersOnTheDerivedKey", func(t *testing.T) {
		// A key column of the joined select is a column of the derived table,
		// named through the select's model reference; the other columns of
		// that model are not.
		_, sel := withTags(left, onRecord)
		rows := make([]recordTags, 0)
		require.NoError(t, sel.Where(TestRecordTagCols.RecordID.Eq("a3")).Scan(&rows))
		require.Equal(t, []recordTags{{ID: "a3", Category: "alpha", Tags: new(int64(1))}}, rows)

		// A column the select does not project is refused with the aliases
		// it does, the way the projection refuses it.
		_, sel = withTags(left, onRecord)
		err := sel.Where(TestRecordTagCols.Label.Eq("vip")).Scan(&rows)
		require.ErrorIs(t, err, database.ErrUnusableFilter)
		require.ErrorIs(t, err, database.ErrJoinSelectColumn)
		require.ErrorContains(t, err, "may name the select's keys, record_id")
		// Tied to by an EqCol as well.
		_, sel = withTags(left, onRecord)
		err = sel.Where(TestAggregateRecordCols.Category.EqCol(TestRecordTagCols.Label)).Scan(&rows)
		require.ErrorIs(t, err, database.ErrJoinSelectColumn)
		require.ErrorContains(t, err, "may name the select's keys, record_id")
	})

	t.Run("ConsumesTheSelectsDryRun", func(t *testing.T) {
		// The query's terminal runs the joined select for real and consumes
		// a dry run set on it, so the select read again on its own executes.
		statements := make([]types.SQLStatement, 0)
		tags, counts := tagCounts(ctx)
		rows := make([]recordTags, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, recordTags](ctx,
			TestAggregateRecordCols.ID, TestAggregateRecordCols.Category, tags).
			Join(types.LeftJoinSelect(counts.WithDryRun(&statements), onRecord())).
			Scan(&rows))
		require.Len(t, rows, 6)
		require.Empty(t, statements)
		perRecord := make([]tagsPerRecord, 0)
		require.NoError(t, counts.Scan(&perRecord))
		require.Len(t, perRecord, 3)

		// Consumed when the query fails to build as well: the option is the
		// query's terminal's to consume, reached or not.
		counts.WithDryRun(&statements)
		type short struct{ ID string }
		require.Error(t, database.Select[*TestAggregateRecord, short](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.LeftJoinSelect(counts, onRecord())).
			Scan(&[]short{}))
		require.NoError(t, counts.Scan(&perRecord))
		require.Len(t, perRecord, 3)
		require.Empty(t, statements)

		// And when the query never attached: the terminal returns at once,
		// consuming on its way out.
		counts.WithDryRun(&statements)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, recordTags](ctx, TestAggregateRecordCols.ID, TestAggregateRecordCols.Category, tags).
			Join(types.LeftJoinSelect(counts, onRecord())).
			WithDryRun(nil).Scan(&rows), database.ErrNilSQLBuilder)
		require.NoError(t, counts.Scan(&perRecord))
		require.Len(t, perRecord, 3)
		require.Empty(t, statements)
	})

	t.Run("AnAliasedTermReadsThroughAnInnerJoin", func(t *testing.T) {
		// Under an inner join the read-through carries no sign of its own:
		// the query's count of its rows would be one per record, the
		// select's is the tags of each, and the aliased term reads the
		// select's.
		n := types.Count().As("n")
		perRecord := database.Select[*TestRecordTag, struct {
			RecordID string
			N        int64
		}](ctx, TestRecordTagCols.RecordID.Group(), n)
		type counted struct {
			ID string
			N  int64
		}
		rows := make([]counted, 0)
		query := database.Select[*TestAggregateRecord, counted](ctx, TestAggregateRecordCols.ID.Group(), n).
			Join(types.JoinSelect(perRecord, onRecord())).
			OrderBy(TestAggregateRecordCols.ID.Group().Asc())
		statements := make([]types.SQLStatement, 0)
		require.NoError(t, query.WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query, qualified("j0", "n")+" AS "+quoteIdent("n"))
		require.NoError(t, query.Scan(&rows))
		require.Equal(t, []counted{{ID: "a1", N: 1}, {ID: "a3", N: 1}, {ID: "a4", N: 1}}, rows)
	})

	t.Run("ChainedSelectsInAnyTermOrder", func(t *testing.T) {
		// The notes are tied on the tags' key; the query groups by that key
		// through the tags' key term, wherever in the projection it stands.
		noteTag := types.NewColumn[*TestTagNote, string]("tag_id")
		notes := types.NewColumn[*TestTagNote, string]("id").Count().As("notes")
		type chained struct {
			ID       string
			RecordID *string
			Tags     *int64
			Notes    *int64
		}
		for _, keyLast := range []bool{false, true} {
			tags, counts := tagCounts(ctx)
			perTag := database.Select[*TestTagNote, struct {
				TagID string
				Notes int64
			}](ctx, noteTag.Group(), notes)
			key := TestRecordTagCols.RecordID.Group()
			exprs := []types.Expr{TestAggregateRecordCols.ID.Group(), key, tags, notes}
			if keyLast {
				exprs = []types.Expr{TestAggregateRecordCols.ID.Group(), tags, notes, key}
			}
			rows := make([]chained, 0)
			require.NoError(t, database.Select[*TestAggregateRecord, chained](ctx, exprs...).
				Join(types.LeftJoinSelect(counts, onRecord()), types.LeftJoinSelect(perTag, noteTag.EqCol(TestRecordTagCols.RecordID))).
				OrderBy(TestAggregateRecordCols.ID.Group().Asc()).
				Scan(&rows))
			require.Len(t, rows, 6)
		}
	})

	t.Run("SelectsMayJoinTheModelsTheQueryJoins", func(t *testing.T) {
		// A select joining a model the query also joins reads that model for
		// itself, as a second select joining it does: the spellings meet on a
		// shared term alone, never on the table.
		tags := TestRecordTagCols.ID.Count().As("tags")
		perCategory := database.Select[*TestRecordTag, struct {
			Category string
			Tags     int64
			Tier     *string
		}](ctx, TestRecordTagCols.Category.Group(), tags, TestAccountCols.Tier.Max().As("tier")).
			Join(types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestRecordTagCols.Category)))
		paid := TestPaymentCols.Amount.Sum().As("paid")
		perAccount := database.Select[*TestPayment, struct {
			Account string
			Paid    int64
			Name    *string
		}](ctx, TestPaymentCols.Account.Group(), paid, TestAccountCols.Name.Max().As("name")).
			Join(types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestPaymentCols.Account)))
		type accounted struct {
			ID   string
			Tier *string
			Tags *int64
			Paid *int64
		}
		statements := make([]types.SQLStatement, 0)
		rows := make([]accounted, 0)
		sel := database.Select[*TestAggregateRecord, accounted](ctx, TestAggregateRecordCols.ID, TestAccountCols.Tier.As("tier"), tags, paid).
			Join(types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestAggregateRecordCols.Category)),
				types.LeftJoinSelect(perCategory, TestRecordTagCols.Category.EqCol(TestAggregateRecordCols.Category)),
				types.LeftJoinSelect(perAccount, TestPaymentCols.Account.EqCol(TestAggregateRecordCols.Category))).
			OrderBy(TestAggregateRecordCols.ID.Asc())
		require.NoError(t, sel.WithDryRun(&statements).Scan(&rows))
		require.Equal(t, 3, strings.Count(statements[0].Query, "LEFT JOIN "+quoteIdent("test_accounts")+" ON "))
		require.NoError(t, sel.Scan(&rows))
		require.Len(t, rows, 6)
	})
}

func TestSelectJoinSelectInAGroupedSelect(t *testing.T) {
	defer cleanupFlowData()
	seedFlowExample()
	ctx := context.Background()

	// Every account's paid total beside its refunded total: the refunds are
	// grouped by account and joined on it, and the query groups by that very
	// column, so the refunded total is constant within a group and reads as
	// a group key of the query.
	type accountFlow struct {
		Account  string
		Paid     int64
		Refunded *int64
	}
	refunded := TestRefundCols.Amount.Sum().As("refunded")
	refunds := database.Select[*TestRefund, struct {
		Account  string
		Refunded int64
	}](ctx, TestRefundCols.Account.Group(), refunded)
	flows := func() types.Selector[*TestPayment, accountFlow] {
		return database.Select[*TestPayment, accountFlow](ctx,
			TestPaymentCols.Account.Group(), TestPaymentCols.Amount.Sum().As("paid"), refunded).
			Join(types.LeftJoinSelect(refunds, TestRefundCols.Account.EqCol(TestPaymentCols.Account))).
			OrderBy(TestPaymentCols.Account.Group().Asc())
	}

	t.Run("ProjectsTheDerivedTermAsAGroupKey", func(t *testing.T) {
		rows := make([]accountFlow, 0)
		require.NoError(t, flows().Scan(&rows))
		require.Equal(t, []accountFlow{
			{Account: "acme", Paid: 700, Refunded: new(int64(50))},
			{Account: "bolt", Paid: 300, Refunded: new(int64(30))},
		}, rows)

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, flows().WithDryRun(&statements).Scan(&rows))
		sql := statements[0].Query
		require.Contains(t, sql, qualified("j0", "refunded")+" AS "+quoteIdent("refunded"))
		require.Contains(t, sql, " GROUP BY "+qualified("test_payments", "account")+","+qualified("j0", "refunded")+" ORDER BY ",
			"the derived term joins the GROUP BY, which is exact because the query groups by the join column")

		total := 0
		require.NoError(t, flows().Count(&total))
		require.Equal(t, 2, total)
	})

	t.Run("HavingReadsTheDerivedTerm", func(t *testing.T) {
		rows := make([]accountFlow, 0)
		require.NoError(t, flows().Having(refunded.Gt(40)).Scan(&rows))
		require.Equal(t, []accountFlow{{Account: "acme", Paid: 700, Refunded: new(int64(50))}}, rows)
	})

	t.Run("RequiresTheQueryToGroupByTheJoinColumn", func(t *testing.T) {
		// Grouped by month, the payments of one group name several accounts,
		// so no single refunded total belongs to the group.
		type monthFlow struct {
			Month    string
			Paid     int64
			Refunded *int64
		}
		rows := make([]monthFlow, 0)
		err := database.Select[*TestPayment, monthFlow](ctx,
			TestPaymentCols.PaidAt.ByMonth().As("month"), TestPaymentCols.Amount.Sum().As("paid"), refunded).
			Join(types.LeftJoinSelect(refunds, TestRefundCols.Account.EqCol(TestPaymentCols.Account))).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrJoinSelectNotKeyed)
	})
}

func TestSelectJoinMixesSources(t *testing.T) {
	defer cleanupJoinData()
	setupJoinData(t)
	ctx := context.Background()
	require.NoError(t, database.Database[*TestRefund](ctx).Create(refundSeed()...))

	// A model and a select joined side by side: every payment with its
	// account's name and its account's refunded total.
	type paymentContext struct {
		ID          string
		AccountName *string
		Refunded    *int64
	}
	refunded := TestRefundCols.Amount.Sum().As("refunded")
	refunds := database.Select[*TestRefund, struct {
		Account  string
		Refunded int64
	}](ctx, TestRefundCols.Account.Group(), refunded)
	sel := database.Select[*TestPayment, paymentContext](ctx,
		TestPaymentCols.ID, TestAccountCols.Name.As("account_name"), refunded).
		Join(
			types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestPaymentCols.Account)),
			types.LeftJoinSelect(refunds, TestRefundCols.Account.EqCol(TestPaymentCols.Account)),
		).
		OrderBy(TestPaymentCols.ID.Asc())
	rows := make([]paymentContext, 0)
	require.NoError(t, sel.Scan(&rows))
	require.Equal(t, []paymentContext{
		{ID: "p1", AccountName: new("Acme Ltd"), Refunded: new(int64(50))},
		{ID: "p2", AccountName: new("Acme Ltd"), Refunded: new(int64(50))},
		{ID: "p3", AccountName: nil, Refunded: new(int64(30))},
		{ID: "p4", AccountName: new("Acme Ltd"), Refunded: new(int64(50))},
	}, rows)

	statements := make([]types.SQLStatement, 0)
	require.NoError(t, sel.WithDryRun(&statements).Scan(&rows))
	sql := statements[0].Query
	require.Contains(t, sql, " LEFT JOIN "+quoteIdent("test_accounts")+" ON ")
	require.Contains(t, sql, " LEFT JOIN (SELECT ")
	require.Contains(t, sql, ") AS "+quoteIdent("j1")+" ON "+qualified("j1", "account")+" = "+qualified("test_payments", "account")+" WHERE ",
		"the derived table takes the alias of its position among the joins")
}

func TestSelectJoinSelectBuildErrors(t *testing.T) {
	ctx := context.Background()
	type recordTags struct {
		ID   string
		Tags *int64
	}
	rows := make([]recordTags, 0)
	onRecord := TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID)

	t.Run("SelectNotGrouped", func(t *testing.T) {
		plain := database.Select[*TestRecordTag, struct {
			RecordID string
			Tags     string
		}](ctx, TestRecordTagCols.RecordID, TestRecordTagCols.Label.As("tags"))
		tags := TestRecordTagCols.Label.As("tags")
		err := database.Select[*TestAggregateRecord, struct {
			ID   string
			Tags *string
		}](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.LeftJoinSelect(plain, onRecord)).
			Scan(&[]struct {
				ID   string
				Tags *string
			}{})
		require.ErrorIs(t, err, database.ErrJoinSelectNotGrouped)
	})

	t.Run("SelectCarryingOrdering", func(t *testing.T) {
		tags, counts := tagCounts(ctx)
		err := database.Select[*TestAggregateRecord, recordTags](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.LeftJoinSelect(counts.OrderBy(tags.Desc()), onRecord)).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrNestedSelectOrdered)
	})

	t.Run("BucketKeyCannotBeJoined", func(t *testing.T) {
		// The select's key is a day label; no time column of the query
		// equals a label, so the join could only ever match nothing.
		type dayTotal struct {
			Day   string
			Total int64
		}
		total := TestPaymentCols.Amount.Sum().As("total")
		perDay := database.Select[*TestPayment, dayTotal](ctx, TestPaymentCols.PaidAt.ByDay().As("day"), total)
		type refundDay struct {
			ID    string
			Total *int64
		}
		err := database.Select[*TestRefund, refundDay](ctx, TestRefundCols.ID, total).
			Join(types.LeftJoinSelect(perDay, TestPaymentCols.PaidAt.EqCol(TestRefundCols.SettledAt))).
			Scan(&[]refundDay{})
		require.ErrorIs(t, err, database.ErrJoinSelectBucketKey)
	})

	t.Run("KeyNotFullyPinned", func(t *testing.T) {
		// Grouped by record and label, the select has two keys; pinning the
		// record alone leaves a record matching one group per label.
		tags := TestRecordTagCols.ID.Count().As("tags")
		perLabel := database.Select[*TestRecordTag, struct {
			RecordID string
			Label    string
			Tags     int64
		}](ctx, TestRecordTagCols.RecordID.Group(), TestRecordTagCols.Label.Group(), tags)
		err := database.Select[*TestAggregateRecord, recordTags](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.LeftJoinSelect(perLabel, onRecord)).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrJoinNotUnique)
		require.ErrorContains(t, err, "record_id")
	})

	t.Run("ModelColumnOfTheJoinedSelect", func(t *testing.T) {
		// The select's model columns are not columns of the derived table:
		// only the terms the select projects are.
		tags, counts := tagCounts(ctx)
		err := database.Select[*TestAggregateRecord, struct {
			ID    string
			Label string
			Tags  *int64
		}](ctx, TestAggregateRecordCols.ID, TestRecordTagCols.Label, tags).
			Join(types.LeftJoinSelect(counts, onRecord)).
			Scan(&[]struct {
				ID    string
				Label string
				Tags  *int64
			}{})
		require.ErrorIs(t, err, database.ErrJoinSelectColumn)
	})

	t.Run("UnionIsNotASelect", func(t *testing.T) {
		_, counts := tagCounts(ctx)
		stacked := database.UnionAll[tagsPerRecord](ctx, counts)
		tags := TestRecordTagCols.ID.Count().As("tags")
		err := database.Select[*TestAggregateRecord, recordTags](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.LeftJoinSelect(stacked, onRecord)).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrJoinSource)
	})

	t.Run("KeyOfATableTheSelectJoins", func(t *testing.T) {
		// The select groups by the record's category beside the tag's; both
		// columns are named category, and the query could name only the
		// tag's. Tied on one, the join would match a record several groups.
		tags := TestRecordTagCols.ID.Count().As("tags")
		perCategories := database.Select[*TestRecordTag, struct {
			TagCat string
			RecCat string
			Tags   int64
		}](ctx, TestRecordTagCols.Category.Group().As("tag_cat"), TestAggregateRecordCols.Category.Group().As("rec_cat"), tags).
			Join(types.Join[*TestAggregateRecord](TestAggregateRecordCols.ID.EqCol(TestRecordTagCols.RecordID)))
		err := database.Select[*TestAggregateRecord, recordTags](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.LeftJoinSelect(perCategories, TestRecordTagCols.Category.EqCol(TestAggregateRecordCols.Category))).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrJoinSelectKey)
		require.ErrorContains(t, err, "test_aggregate_records")
	})

	t.Run("KeyColumnGroupedTwice", func(t *testing.T) {
		// Two keys on one column would be one reference naming both: the
		// second would shadow the first, and the join match several groups.
		tags := TestRecordTagCols.ID.Count().As("tags")
		twice := database.Select[*TestRecordTag, struct {
			A    string
			B    string
			Tags int64
		}](ctx, TestRecordTagCols.RecordID.Group().As("a"), TestRecordTagCols.RecordID.Group().As("b"), tags)
		err := database.Select[*TestAggregateRecord, recordTags](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.LeftJoinSelect(twice, onRecord)).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrJoinSelectKey)
		require.ErrorContains(t, err, "twice")
	})

	t.Run("ExistsInsideTheOn", func(t *testing.T) {
		// The derived table's rows are the select's groups, which no
		// subquery can correlate with; the condition belongs to the select's
		// own Where.
		tags, counts := tagCounts(ctx)
		noteTag := types.NewColumn[*TestTagNote, string]("tag_id")
		err := database.Select[*TestAggregateRecord, recordTags](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.LeftJoinSelect(counts, onRecord, types.FilterExists[*TestTagNote](noteTag.EqCol(TestRecordTagCols.RecordID)))).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrUnusableFilter)
		require.ErrorContains(t, err, "its own Where")
	})

	t.Run("TermOfTheQueriedTableReadThroughASelect", func(t *testing.T) {
		// The select reads the records through a select of its own; a term
		// of the records it projects is spelled like the query's own, and
		// that spelling is refused rather than read as either.
		recordID := TestAggregateRecordCols.ID.Group()
		perRecord := database.Select[*TestAggregateRecord, struct {
			ID    string
			Total int64
		}](ctx, recordID, TestAggregateRecordCols.Amount.Sum().As("total"))
		tags := TestRecordTagCols.ID.Count().As("tags")
		perTag := database.Select[*TestRecordTag, struct {
			RecordID string
			Tags     int64
			ID       *string
		}](ctx, TestRecordTagCols.RecordID.Group(), tags, recordID).
			Join(types.LeftJoinSelect(perRecord, TestAggregateRecordCols.ID.EqCol(TestRecordTagCols.RecordID)))
		type keyed struct {
			ID   *string
			Tags *int64
		}
		err := database.Select[*TestAggregateRecord, keyed](ctx, recordID, tags).
			Join(types.LeftJoinSelect(perTag, onRecord)).
			Scan(&[]keyed{})
		require.ErrorIs(t, err, database.ErrDuplicateAlias)
		require.ErrorContains(t, err, "under its default alias")
		// The query's own column, spelled as its own, reads as its own.
		statements := make([]types.SQLStatement, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, recordTags](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.LeftJoinSelect(perTag, onRecord)).
			WithDryRun(&statements).Scan(&rows))
		require.Contains(t, statements[0].Query, "SELECT "+qualified("test_aggregate_records", "id")+" AS ")
	})

	t.Run("OwnCountSharedWithTheSelect", func(t *testing.T) {
		// A plain Count carries no table: written by the query and by the
		// select, it is one value, and the query's own count would silently
		// read as the select's.
		perRecord := database.Select[*TestRecordTag, struct {
			RecordID string
			Count    int64
		}](ctx, TestRecordTagCols.RecordID.Group(), types.Count())
		type counted struct {
			ID    string
			Count *int64
		}
		err := database.Select[*TestAggregateRecord, counted](ctx, TestAggregateRecordCols.ID.Group(), types.Count()).
			Join(types.LeftJoinSelect(perRecord, onRecord)).
			Scan(&[]counted{})
		require.ErrorIs(t, err, database.ErrDuplicateAlias)
		require.ErrorContains(t, err, "under its default alias")
	})

	t.Run("SharedConstantIsRefused", func(t *testing.T) {
		// A constant is the query's own under any alias: written by the
		// select too, it is refused rather than read as either, and whether
		// a row matched is read from the select's key.
		kind := types.Literal("tagged").As("kind")
		perRecord := database.Select[*TestRecordTag, struct {
			RecordID string
			Kind     string
		}](ctx, TestRecordTagCols.RecordID.Group(), kind)
		type marked struct {
			ID   string
			Kind *string
		}
		err := database.Select[*TestAggregateRecord, marked](ctx, TestAggregateRecordCols.ID, kind).
			Join(types.LeftJoinSelect(perRecord, onRecord)).
			Scan(&[]marked{})
		require.ErrorIs(t, err, database.ErrDuplicateAlias)
		require.ErrorContains(t, err, "a constant is the query's own")
	})

	t.Run("TermUnderItsDefaultAliasHoweverSpelled", func(t *testing.T) {
		// The default alias is the effective one: the two sides spelled
		// independently — never aliased, aliased to nothing, aliased to the
		// default name, built by hand without one — are refused alike.
		for _, tt := range []struct {
			label    string
			own, sub types.Term
		}{
			{"NeverAliased", TestAccountCols.Tier.Max(), TestAccountCols.Tier.Max()},
			{"AliasedToNothing", TestAccountCols.Tier.Max().As(""), TestAccountCols.Tier.Max()},
			{"AliasedToTheDefault", TestAccountCols.Tier.Max().As("tier"), TestAccountCols.Tier.Max().As("")},
			{"BuiltByHand", types.Term{Fn: types.FnMax, Table: "test_accounts", Column: "tier"}, TestAccountCols.Tier.Max()},
		} {
			t.Run(tt.label, func(t *testing.T) {
				perCategory := database.Select[*TestRecordTag, struct {
					Category string
					Tier     *string
				}](ctx, TestRecordTagCols.Category.Group(), tt.sub).
					Join(types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestRecordTagCols.Category)))
				type tiered struct {
					ID   string
					Tier *string
				}
				err := database.Select[*TestAggregateRecord, tiered](ctx, TestAggregateRecordCols.ID, tt.own).
					Join(types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestAggregateRecordCols.Category)),
						types.LeftJoinSelect(perCategory, TestRecordTagCols.Category.EqCol(TestAggregateRecordCols.Category))).
					Scan(&[]tiered{})
				require.ErrorIs(t, err, database.ErrDuplicateAlias)
				require.ErrorContains(t, err, "under its default alias")
			})
		}
	})

	t.Run("AliasedTermsReadThrough", func(t *testing.T) {
		// An alias is written on purpose: the select's aliased Count is read
		// through, as is the aliased term of a model the query joins itself.
		n := types.Count().As("n")
		perRecord := database.Select[*TestRecordTag, struct {
			RecordID string
			N        int64
		}](ctx, TestRecordTagCols.RecordID.Group(), n)
		type marked struct {
			ID string
			N  *int64
		}
		statements := make([]types.SQLStatement, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, marked](ctx, TestAggregateRecordCols.ID, n).
			Join(types.LeftJoinSelect(perRecord, onRecord)).
			WithDryRun(&statements).Scan(&[]marked{}))
		require.Contains(t, statements[0].Query, qualified("j0", "n")+" AS "+quoteIdent("n"))

		tier := TestAccountCols.Tier.Max().As("top_tier")
		perCategory := database.Select[*TestRecordTag, struct {
			Category string
			TopTier  *string
		}](ctx, TestRecordTagCols.Category.Group(), tier).
			Join(types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestRecordTagCols.Category)))
		type tiered struct {
			ID      string
			Name    *string
			TopTier *string
		}
		statements = statements[:0]
		require.NoError(t, database.Select[*TestAggregateRecord, tiered](ctx, TestAggregateRecordCols.ID, TestAccountCols.Name.As("name"), tier).
			Join(types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestAggregateRecordCols.Category)),
				types.LeftJoinSelect(perCategory, TestRecordTagCols.Category.EqCol(TestAggregateRecordCols.Category))).
			WithDryRun(&statements).Scan(&[]tiered{}))
		require.Contains(t, statements[0].Query, qualified("j1", "top_tier")+" AS "+quoteIdent("top_tier"))
	})

	t.Run("AJoinedModelsTermKeepsItsOwnAlias", func(t *testing.T) {
		// The query joins the accounts itself and reads the select's aliased
		// term of them beside its own, under an alias of its own: the two are
		// the query's and the select's, told apart by alias.
		subTier := TestAccountCols.Tier.Max().As("sub_tier")
		perCategory := database.Select[*TestRecordTag, struct {
			Category string
			SubTier  *string
		}](ctx, TestRecordTagCols.Category.Group(), subTier).
			Join(types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestRecordTagCols.Category)))
		ownTier := TestAccountCols.Tier.Max().As("own_tier")
		type tiered struct {
			Category string
			OwnTier  *string
			SubTier  *string
		}
		statements := make([]types.SQLStatement, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, tiered](ctx, TestAggregateRecordCols.Category.Group(), ownTier, subTier).
			Join(types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestAggregateRecordCols.Category)),
				types.LeftJoinSelect(perCategory, TestRecordTagCols.Category.EqCol(TestAggregateRecordCols.Category))).
			WithDryRun(&statements).Scan(&[]tiered{}))
		require.Contains(t, statements[0].Query, "MAX("+qualified("test_accounts", "tier")+") AS "+quoteIdent("own_tier"))
		require.Contains(t, statements[0].Query, qualified("j1", "sub_tier")+" AS "+quoteIdent("sub_tier"))
	})

	t.Run("SelectJoinedIntoItself", func(t *testing.T) {
		// A select joined into itself, directly or through another, is
		// refused rather than described without end.
		tags, counts := tagCounts(ctx)
		counts.Join(types.LeftJoinSelect(counts, TestRecordTagCols.RecordID.EqCol(TestRecordTagCols.RecordID)))
		err := counts.Scan(&[]tagsPerRecord{})
		require.ErrorIs(t, err, database.ErrJoinDuplicateTable)
		require.ErrorContains(t, err, "joins itself")

		noteTag := types.NewColumn[*TestTagNote, string]("tag_id")
		notes := types.NewColumn[*TestTagNote, string]("id").Count().As("notes")
		perTag := database.Select[*TestTagNote, struct {
			TagID string
			Notes int64
		}](ctx, noteTag.Group(), notes)
		_, perRecord := tagCounts(ctx)
		perRecord.Join(types.LeftJoinSelect(perTag, noteTag.EqCol(TestRecordTagCols.RecordID)))
		perTag.Join(types.LeftJoinSelect(perRecord, TestRecordTagCols.RecordID.EqCol(noteTag)))
		err = database.Select[*TestAggregateRecord, recordTags](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.LeftJoinSelect(perRecord, onRecord)).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrJoinDuplicateTable)
		require.ErrorContains(t, err, "joins itself")
	})

	t.Run("NestedKeyProvesNoOtherSource", func(t *testing.T) {
		// A key of the notes read through the tags names the tags' derived
		// column; it does not prove the query groups by the notes' own key,
		// which another select is tied to.
		noteTag := types.NewColumn[*TestTagNote, string]("tag_id")
		noteID := types.NewColumn[*TestTagNote, string]("id")
		perNote := database.Select[*TestTagNote, struct {
			Nt string
			Un int64
		}](ctx, noteTag.Group().As("nt"), noteID.Count().As("un"))
		perTag := database.Select[*TestTagNote, struct {
			TagID string
			Notes int64
		}](ctx, noteTag.Group(), noteID.Count().As("notes"))
		perRecordTag := database.Select[*TestRecordTag, struct {
			ID    string
			TagID *string
		}](ctx, TestRecordTagCols.ID.Group(), noteTag.Group()).
			Join(types.LeftJoinSelect(perTag, noteTag.EqCol(TestRecordTagCols.ID)))
		paid := TestPaymentCols.Amount.Sum().As("paid")
		perAccount := database.Select[*TestPayment, struct {
			Account string
			Paid    int64
		}](ctx, TestPaymentCols.Account.Group(), paid)
		type out struct {
			Category string
			TagID    *string
			Paid     *int64
		}
		err := database.Select[*TestAggregateRecord, out](ctx,
			TestAggregateRecordCols.Category.Group(), noteTag.Group(), paid).
			Join(types.LeftJoinSelect(perNote, noteTag.EqCol(TestAggregateRecordCols.ID)),
				types.LeftJoinSelect(perRecordTag, TestRecordTagCols.ID.EqCol(TestAggregateRecordCols.Category)),
				types.LeftJoinSelect(perAccount, TestPaymentCols.Account.EqCol(noteTag))).
			Scan(&[]out{})
		require.ErrorIs(t, err, database.ErrJoinSelectNotKeyed)
		require.ErrorContains(t, err, `key term "nt"`)
	})

	t.Run("OnTiedToANonKeyOfAnotherSelect", func(t *testing.T) {
		// A select tied to a column another select's model has, but does not
		// key, is refused before the grouping is proved, in either shape.
		tags, counts := tagCounts(ctx)
		noteTag := types.NewColumn[*TestTagNote, string]("tag_id")
		notes := types.NewColumn[*TestTagNote, string]("id").Count().As("notes")
		perTag := database.Select[*TestTagNote, struct {
			TagID string
			Notes int64
		}](ctx, noteTag.Group(), notes)
		type noted struct {
			ID    string
			Tags  *int64
			Notes *int64
		}
		for _, key := range []types.Expr{TestAggregateRecordCols.ID.Group(), TestAggregateRecordCols.ID} {
			err := database.Select[*TestAggregateRecord, noted](ctx, key, tags, notes).
				Join(types.LeftJoinSelect(counts, onRecord), types.LeftJoinSelect(perTag, noteTag.EqCol(TestRecordTagCols.Label))).
				Scan(&[]noted{})
			require.ErrorIs(t, err, database.ErrJoinSelectColumn)
			require.ErrorContains(t, err, "is not a key of that joined select")
		}
	})

	t.Run("NonKeyColumnInTheOn", func(t *testing.T) {
		// The ON names the select's keys; a column of its model that is no
		// key is refused with the keys, as it is in Where.
		tags, counts := tagCounts(ctx)
		err := database.Select[*TestAggregateRecord, recordTags](ctx, TestAggregateRecordCols.ID, tags).
			Join(types.LeftJoinSelect(counts, onRecord, TestRecordTagCols.Label.Eq("vip"))).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrJoinSelectColumn)
		require.ErrorContains(t, err, "may name the select's keys, record_id")
	})

	t.Run("PartitionByATermTheProjectionDoesNotRead", func(t *testing.T) {
		// A select's term keys a partition once the projection reads it;
		// unread, the message says to project it.
		tags, counts := tagCounts(ctx)
		type numbered struct {
			ID string
			Rn int64
		}
		err := database.Select[*TestAggregateRecord, numbered](ctx, TestAggregateRecordCols.ID,
			types.RowNumber().Over(types.PartitionBy(tags).OrderBy(TestAggregateRecordCols.ID.Asc())).As("rn")).
			Join(types.LeftJoinSelect(counts, onRecord)).
			Scan(&[]numbered{})
		require.ErrorIs(t, err, database.ErrWindowTermNotSelected)
		require.ErrorContains(t, err, "project it")
	})

	t.Run("TermUnderAnotherAlias", func(t *testing.T) {
		// The select's term re-aliased is not the term it projects; naming
		// the mistake here keeps the error beside it.
		tags, counts := tagCounts(ctx)
		type renamed struct {
			ID string
			X  *int64
		}
		err := database.Select[*TestAggregateRecord, renamed](ctx, TestAggregateRecordCols.ID, tags.As("x")).
			Join(types.LeftJoinSelect(counts, onRecord)).
			Scan(&[]renamed{})
		require.ErrorIs(t, err, database.ErrJoinSelectColumn)
		require.ErrorContains(t, err, "under another alias")
	})

	t.Run("TermOfATableOnlyTheSelectJoins", func(t *testing.T) {
		// The query does not join the accounts; the select does, and its
		// term of them renamed is not the term it projects.
		topTier := TestAccountCols.Tier.Max().As("top_tier")
		perCategory := database.Select[*TestRecordTag, struct {
			Category string
			TopTier  *string
		}](ctx, TestRecordTagCols.Category.Group(), topTier).
			Join(types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestRecordTagCols.Category)))
		type renamed struct {
			ID   string
			Tier *string
		}
		err := database.Select[*TestAggregateRecord, renamed](ctx, TestAggregateRecordCols.ID, TestAccountCols.Tier.Max().As("tier")).
			Join(types.LeftJoinSelect(perCategory, TestRecordTagCols.Category.EqCol(TestAggregateRecordCols.Category))).
			Scan(&[]renamed{})
		require.ErrorIs(t, err, database.ErrJoinSelectColumn)
		require.ErrorContains(t, err, `"tier" is the term "top_tier" of the joined select over "test_record_tags" altered`)
	})

	t.Run("TermOfTheSecondJoinedSelectUnderAnotherAlias", func(t *testing.T) {
		// Every joined select is asked: the term the second one projects,
		// renamed by the query, is named as that select's.
		tags, counts := tagCounts(ctx)
		top := TestRecordTagCols.Label.Max().As("top")
		noteTag := types.NewColumn[*TestTagNote, string]("tag_id")
		perTag := database.Select[*TestTagNote, struct {
			RecordID string
			Top      *string
		}](ctx, noteTag.Group().As("record_id"), top).
			Join(types.LeftJoin[*TestRecordTag](TestRecordTagCols.ID.EqCol(noteTag)))
		type renamed struct {
			ID   string
			Tags *int64
			Best *string
		}
		err := database.Select[*TestAggregateRecord, renamed](ctx, TestAggregateRecordCols.ID, tags, TestRecordTagCols.Label.Max().As("best")).
			Join(types.LeftJoinSelect(counts, onRecord), types.LeftJoinSelect(perTag, noteTag.EqCol(TestAggregateRecordCols.ID))).
			Scan(&[]renamed{})
		require.ErrorIs(t, err, database.ErrJoinSelectColumn)
		require.ErrorContains(t, err, `"best" is the term "top" of the joined select over "test_tag_notes"`)
	})

	t.Run("WindowOverADerivedTermIsRefused", func(t *testing.T) {
		// The select's term windowed or conditioned is not the term the
		// select projects. Carrying no table, the query could compute it
		// itself, and under the select's alias it would silently count the
		// query's rows instead of reading the select's count; a term of the
		// select's own table is its term altered.
		n := types.Count().As("n")
		perRecord := database.Select[*TestRecordTag, struct {
			RecordID string
			N        int64
		}](ctx, TestRecordTagCols.RecordID.Group(), n)
		type counted struct {
			ID string
			N  *int64
		}
		err := database.Select[*TestAggregateRecord, counted](ctx, TestAggregateRecordCols.ID, n.Over(types.PartitionBy(TestAggregateRecordCols.Category))).
			Join(types.LeftJoinSelect(perRecord, onRecord)).
			Scan(&[]counted{})
		require.ErrorIs(t, err, database.ErrDuplicateAlias)
		require.ErrorContains(t, err, "with a window or conditions of its own")
		err = database.Select[*TestAggregateRecord, counted](ctx, TestAggregateRecordCols.ID, n.Where(TestAggregateRecordCols.Status.Eq("done"))).
			Join(types.LeftJoinSelect(perRecord, onRecord)).
			Scan(&[]counted{})
		require.ErrorIs(t, err, database.ErrDuplicateAlias)
		tags, counts := tagCounts(ctx)
		err = database.Select[*TestAggregateRecord, recordTags](ctx, TestAggregateRecordCols.ID, tags.Over(types.PartitionBy(TestAggregateRecordCols.Category))).
			Join(types.LeftJoinSelect(counts, onRecord)).
			Scan(&rows)
		require.ErrorIs(t, err, database.ErrJoinSelectColumn)
		require.ErrorContains(t, err, "altered")
	})

	t.Run("TermProjectedByTwoJoinedSelects", func(t *testing.T) {
		// One MAX(label) shared by two selects, the notes reading the tags
		// through a join of their own, would read from either; the query has
		// to alias them apart.
		top := TestRecordTagCols.Label.Max().As("top")
		noteTag := types.NewColumn[*TestTagNote, string]("tag_id")
		type keyed struct {
			RecordID string
			Top      *string
		}
		perRecord := database.Select[*TestRecordTag, keyed](ctx, TestRecordTagCols.RecordID.Group(), top)
		perTag := database.Select[*TestTagNote, keyed](ctx, noteTag.Group().As("record_id"), top).
			Join(types.LeftJoin[*TestRecordTag](TestRecordTagCols.ID.EqCol(noteTag)))
		type topped struct {
			ID  string
			Top *string
		}
		err := database.Select[*TestAggregateRecord, topped](ctx, TestAggregateRecordCols.ID, top).
			Join(types.LeftJoinSelect(perRecord, onRecord), types.LeftJoinSelect(perTag, noteTag.EqCol(TestAggregateRecordCols.ID))).
			Scan(&[]topped{})
		require.ErrorIs(t, err, database.ErrDuplicateAlias)
		require.ErrorContains(t, err, "two joined selects")
	})

	t.Run("PartitionByAColumnTheSelectMeasured", func(t *testing.T) {
		// The select counts the tag ids; the query cannot partition by the id
		// column that count read, which is no column of the derived table.
		tags, counts := tagCounts(ctx)
		type share struct {
			ID    string
			Tags  *int64
			Share int64
		}
		err := database.Select[*TestAggregateRecord, share](ctx,
			TestAggregateRecordCols.ID.Group(), tags,
			TestAggregateRecordCols.Amount.Sum().Over(types.PartitionBy(TestRecordTagCols.ID)).As("share")).
			Join(types.LeftJoinSelect(counts, onRecord)).
			Scan(&[]share{})
		require.ErrorIs(t, err, database.ErrWindowTermNotSelected)
	})

	t.Run("ScanOneRefusesADerivedTerm", func(t *testing.T) {
		tags, counts := tagCounts(ctx)
		one := struct {
			Total int64
			Tags  *int64
		}{}
		err := database.Select[*TestAggregateRecord, struct {
			Total int64
			Tags  *int64
		}](ctx, TestAggregateRecordCols.Amount.Sum().As("total"), tags).
			Join(types.LeftJoinSelect(counts, onRecord)).
			ScanOne(&one)
		require.ErrorIs(t, err, database.ErrScanOneRowLevel)
	})
}
