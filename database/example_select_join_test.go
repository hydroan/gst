package database_test

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
)

// The join examples read the seeded payments beside the one seeded account
// (see paymentSeed and accountSeed in fixture_test.go), and the seeded tags
// beside their records (see tagSeed and aggregateSeed). They are written the
// way project code is written, through the generated Cols vars the fixture
// mirrors. The SQL each one renders is quoted in MySQL spelling.

// Join reads another model's columns beside the queried model's: every tag
// with the category and amount of the record it marks. The predicate pins the
// record's primary key, which is what proves a tag matches one record, and
// the record's soft-delete condition is written into the ON. Every column is
// qualified by its table because two tables are read. Rendered:
//
//	SELECT `test_record_tags`.`id` AS `id`, `test_record_tags`.`label` AS `label`,
//	       `test_aggregate_records`.`category` AS `category`, `test_aggregate_records`.`amount` AS `amount`
//	FROM `test_record_tags`
//	JOIN `test_aggregate_records` ON `test_aggregate_records`.`id` = `test_record_tags`.`record_id`
//	                              AND `test_aggregate_records`.`deleted_at` IS NULL
//	WHERE `test_record_tags`.`deleted_at` IS NULL ORDER BY `id` ASC
func ExampleSelect_join() {
	seedAggregateExample()
	defer cleanupAggregateData()
	seedTagExample()
	defer cleanupTagData()

	type taggedRecord struct {
		ID       string
		Label    string
		Category string
		Amount   int64
	}
	rows := make([]taggedRecord, 0)
	if err := database.Select[*TestRecordTag, taggedRecord](context.Background(),
		TestRecordTagCols.ID, TestRecordTagCols.Label, TestAggregateRecordCols.Category, TestAggregateRecordCols.Amount).
		Join(types.Join[*TestAggregateRecord](TestAggregateRecordCols.ID.EqCol(TestRecordTagCols.RecordID))).
		OrderBy(TestRecordTagCols.ID.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.ID, r.Label, r.Category, r.Amount)
	}
	// Output:
	// t1 vip alpha 100
	// t2 vip alpha 300
	// t3 bulk beta 400
}

// LeftJoin keeps the rows that match no joined row, with the joined columns
// NULL: every payment with its account's name, the payment of an account the
// table does not carry included. The account's code is unique through its
// Indexes method, which proves the join. A field a LEFT JOIN can leave NULL
// must be a pointer. Rendered:
//
//	SELECT `test_payments`.`id` AS `id`, `test_payments`.`amount` AS `amount`, `test_accounts`.`name` AS `account_name`
//	FROM `test_payments`
//	LEFT JOIN `test_accounts` ON `test_accounts`.`code` = `test_payments`.`account`
//	                          AND `test_accounts`.`deleted_at` IS NULL
//	WHERE `test_payments`.`deleted_at` IS NULL ORDER BY `id` ASC
func ExampleSelect_leftJoin() {
	seedFlowExample()
	defer cleanupFlowData()
	seedAccountExample()
	defer cleanupAccountData()

	type paymentWithAccount struct {
		ID          string
		Amount      int64
		AccountName *string
	}
	rows := make([]paymentWithAccount, 0)
	if err := database.Select[*TestPayment, paymentWithAccount](context.Background(),
		TestPaymentCols.ID, TestPaymentCols.Amount, TestAccountCols.Name.As("account_name")).
		Join(types.LeftJoin[*TestAccount](TestAccountCols.Code.EqCol(TestPaymentCols.Account))).
		OrderBy(TestPaymentCols.ID.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		name := "(no account)"
		if r.AccountName != nil {
			name = *r.AccountName
		}
		fmt.Println(r.ID, r.Amount, name)
	}
	// Output:
	// p1 100 Acme Ltd
	// p2 200 Acme Ltd
	// p3 300 (no account)
	// p4 400 Acme Ltd
}

// A joined column filters and groups like the queried model's own: the
// payments of gold accounts, totaled per account with the account's name as
// a second group key. In a grouped projection a joined column may be a group
// key or carry MIN, MAX or COUNT DISTINCT, never SUM or COUNT, which would
// count the one account once per payment. Rendered:
//
//	SELECT `test_payments`.`account` AS `account`, `test_accounts`.`name` AS `name`,
//	       COALESCE(SUM(`test_payments`.`amount`), 0) AS `amount`
//	FROM `test_payments`
//	JOIN `test_accounts` ON `test_accounts`.`code` = `test_payments`.`account` AND `test_accounts`.`deleted_at` IS NULL
//	WHERE `test_accounts`.`tier` = ? AND `test_payments`.`deleted_at` IS NULL
//	GROUP BY `test_payments`.`account`,`test_accounts`.`name` ORDER BY `account` ASC
func ExampleSelect_joinGrouped() {
	seedFlowExample()
	defer cleanupFlowData()
	seedAccountExample()
	defer cleanupAccountData()

	type accountTotal struct {
		Account string
		Name    string
		Amount  int64
	}
	rows := make([]accountTotal, 0)
	if err := database.Select[*TestPayment, accountTotal](context.Background(),
		TestPaymentCols.Account.Group(), TestAccountCols.Name.Group(), TestPaymentCols.Amount.Sum()).
		Join(types.Join[*TestAccount](TestAccountCols.Code.EqCol(TestPaymentCols.Account))).
		Where(TestAccountCols.Tier.Eq("gold")).
		OrderBy(TestPaymentCols.Account.Group().Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Account, r.Name, r.Amount)
	}
	// Output:
	// acme Acme Ltd 700
}

// JoinSelect reads a one-to-many relation beside its one side: the tags are
// counted per record first, so every record matches one group at most, and
// the count is read back through the very term the grouped select projects.
// LeftJoinSelect keeps the records without tags, their count NULL. Rendered:
//
//	SELECT `test_aggregate_records`.`id` AS `id`, `test_aggregate_records`.`category` AS `category`, `j0`.`tags` AS `tags`
//	FROM `test_aggregate_records`
//	LEFT JOIN (SELECT `record_id` AS `record_id`, COUNT(`id`) AS `tags`
//	           FROM `test_record_tags` WHERE `test_record_tags`.`deleted_at` IS NULL GROUP BY `record_id`) AS `j0`
//	  ON `j0`.`record_id` = `test_aggregate_records`.`id`
//	WHERE `test_aggregate_records`.`deleted_at` IS NULL ORDER BY `id` ASC
func ExampleSelect_joinSelect() {
	seedAggregateExample()
	defer cleanupAggregateData()
	seedTagExample()
	defer cleanupTagData()

	type tagsPerRecord struct {
		RecordID string
		Tags     int64
	}
	tags := TestRecordTagCols.ID.Count().As("tags")
	counts := database.Select[*TestRecordTag, tagsPerRecord](context.Background(), TestRecordTagCols.RecordID.Group(), tags)

	type recordWithTags struct {
		ID       string
		Category string
		Tags     *int64
	}
	rows := make([]recordWithTags, 0)
	if err := database.Select[*TestAggregateRecord, recordWithTags](context.Background(),
		TestAggregateRecordCols.ID, TestAggregateRecordCols.Category, tags).
		Join(types.LeftJoinSelect(counts, TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID))).
		OrderBy(TestAggregateRecordCols.ID.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		count := "-"
		if r.Tags != nil {
			count = strconv.FormatInt(*r.Tags, 10)
		}
		fmt.Println(r.ID, r.Category, count)
	}
	// Output:
	// a1 alpha 1
	// a2 alpha -
	// a3 alpha 1
	// a4 beta 1
	// a5 beta -
	// a6 gamma -
}

// A grouped query reads a joined select's term as a group key: every account's
// paid total beside its refunded total, the refunds grouped by account and
// joined on it. That is exact because the query groups by the very column
// the select is joined on, which the framework requires. Rendered:
//
//	SELECT `test_payments`.`account` AS `account`, COALESCE(SUM(`test_payments`.`amount`), 0) AS `paid`, `j0`.`refunded` AS `refunded`
//	FROM `test_payments`
//	LEFT JOIN (SELECT `account` AS `account`, COALESCE(SUM(`amount`), 0) AS `refunded`
//	           FROM `test_refunds` WHERE `test_refunds`.`deleted_at` IS NULL GROUP BY `account`) AS `j0`
//	  ON `j0`.`account` = `test_payments`.`account`
//	WHERE `test_payments`.`deleted_at` IS NULL
//	GROUP BY `test_payments`.`account`,`j0`.`refunded` ORDER BY `account` ASC
func ExampleSelect_joinSelectGrouped() {
	seedFlowExample()
	defer cleanupFlowData()

	type accountRefunds struct {
		Account  string
		Refunded int64
	}
	refunded := TestRefundCols.Amount.Sum().As("refunded")
	refunds := database.Select[*TestRefund, accountRefunds](context.Background(), TestRefundCols.Account.Group(), refunded)

	type accountFlow struct {
		Account  string
		Paid     int64
		Refunded *int64
	}
	rows := make([]accountFlow, 0)
	if err := database.Select[*TestPayment, accountFlow](context.Background(),
		TestPaymentCols.Account.Group(), TestPaymentCols.Amount.Sum().As("paid"), refunded).
		Join(types.LeftJoinSelect(refunds, TestRefundCols.Account.EqCol(TestPaymentCols.Account))).
		OrderBy(TestPaymentCols.Account.Group().Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Account, r.Paid, *r.Refunded)
	}
	// Output:
	// acme 700 50
	// bolt 300 30
}
