package database_test

import (
	"context"
	"fmt"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
)

// The union examples stack the seeded payments and refunds (see paymentSeed
// and refundSeed in fixture_test.go) into one flow, and are written the way
// project code is written, through the generated Cols vars the fixture
// mirrors. The SQL each one renders is quoted in MySQL spelling; the other
// dialects differ only in the identifier quotes.

// flow is the row a payment and a refund both scan into. Kind is the tag the
// branch writes into its rows, payment or refund. At is the payment's paid_at
// or the refund's settled_at: the two models spell it differently, and As
// aligns both on the row's own name.
type flow struct {
	Kind    string
	ID      string
	Account string
	Amount  int64
	At      time.Time
}

// paymentFlows and refundFlows are the two branches every flow example
// stacks, kept the way a service keeps them: each is an ordinary select
// scanning into flow, tagged by its kind.
func paymentFlows(ctx context.Context) types.Selector[*TestPayment, flow] {
	return database.Select[*TestPayment, flow](ctx,
		types.Literal("payment").As("kind"),
		TestPaymentCols.ID, TestPaymentCols.Account, TestPaymentCols.Amount,
		TestPaymentCols.PaidAt.As("at"))
}

func refundFlows(ctx context.Context) types.Selector[*TestRefund, flow] {
	return database.Select[*TestRefund, flow](ctx,
		types.Literal("refund").As("kind"),
		TestRefundCols.ID, TestRefundCols.Account, TestRefundCols.Amount,
		TestRefundCols.SettledAt.As("at"))
}

// UnionAll stacks the rows of several selects into one result: here every
// payment and every refund as one flow, newest first. Each branch is an
// ordinary select scanning into the same row type; Literal tags the rows with
// the branch they come from, and the framework renders every branch's SELECT
// list in the row type's field order, so the columns line up by name rather
// than by the position they were written in. The flow is ordered by the term
// the payments project their time under, which names the row's at column.
// Rendered:
//
//	SELECT * FROM (
//	  SELECT * FROM (SELECT 'payment' AS `kind`, `id` AS `id`, `account` AS `account`, `amount` AS `amount`, `paid_at` AS `at`
//	                 FROM `test_payments` WHERE `test_payments`.`deleted_at` IS NULL) AS `b0`
//	  UNION ALL
//	  SELECT * FROM (SELECT 'refund' AS `kind`, `id` AS `id`, `account` AS `account`, `amount` AS `amount`, `settled_at` AS `at`
//	                 FROM `test_refunds` WHERE `test_refunds`.`deleted_at` IS NULL) AS `b1`
//	) AS `u` ORDER BY `at` DESC,`id` DESC
func ExampleUnionAll() {
	seedFlowExample()
	defer cleanupFlowData()
	ctx := context.Background()

	rows := make([]flow, 0)
	if err := database.UnionAll[flow](ctx, paymentFlows(ctx), refundFlows(ctx)).
		OrderBy(TestPaymentCols.PaidAt.As("at").Desc(), TestPaymentCols.ID.Desc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.At.Format("2006-01-02 15:04"), r.Kind, r.ID, r.Account, r.Amount)
	}
	// Output:
	// 2024-02-02 09:00 refund r2 bolt 30
	// 2024-02-01 08:00 payment p4 acme 400
	// 2024-01-12 10:00 payment p3 bolt 300
	// 2024-01-11 12:00 refund r1 acme 50
	// 2024-01-11 09:00 payment p2 acme 200
	// 2024-01-10 08:00 payment p1 acme 100
}

// A flow pages like a select, and with a Page the framework pushes the
// ordering and the page's end, offset plus limit, into every branch: each branch reads its
// first four rows by its own index, and the union sorts eight rows at most to
// take the page. Rendered, with 4 bound inside each member and 2 and 2
// outside:
//
//	SELECT * FROM (
//	  SELECT * FROM (SELECT 'payment' AS `kind`, ... FROM `test_payments` WHERE ...
//	                 ORDER BY `at` DESC,`id` DESC LIMIT ?) AS `b0`
//	  UNION ALL
//	  SELECT * FROM (SELECT 'refund' AS `kind`, ... FROM `test_refunds` WHERE ...
//	                 ORDER BY `at` DESC,`id` DESC LIMIT ?) AS `b1`
//	) AS `u` ORDER BY `at` DESC,`id` DESC LIMIT ? OFFSET ?
func ExampleUnionAll_pagination() {
	seedFlowExample()
	defer cleanupFlowData()
	ctx := context.Background()

	page := make([]flow, 0)
	if err := database.UnionAll[flow](ctx, paymentFlows(ctx), refundFlows(ctx)).
		OrderBy(TestPaymentCols.PaidAt.As("at").Desc(), TestPaymentCols.ID.Desc()).
		Page(2, 2).
		Scan(&page); err != nil {
		panic(err)
	}
	for _, r := range page {
		fmt.Println(r.At.Format("2006-01-02 15:04"), r.Kind, r.ID)
	}
	// Output:
	// 2024-01-12 10:00 payment p3
	// 2024-01-11 12:00 refund r1
}

// Count adds up the counts of the branches and materializes no stacked row;
// like a select's Count it ignores the ordering and paging set on the union,
// which is what the total of a paginated flow needs. A branch keeps its own
// Where, so one account's payments stack with every refund. Rendered:
//
//	SELECT `n` FROM (SELECT (SELECT COUNT(*) FROM (SELECT 1 AS `row_marker` FROM `test_payments` WHERE `account` = ? AND ...) AS `b0`)
//	                    + (SELECT COUNT(*) FROM (SELECT 1 AS `row_marker` FROM `test_refunds` WHERE ...) AS `b1`) AS `n`) AS `counts`
func ExampleUnionAll_count() {
	seedFlowExample()
	defer cleanupFlowData()
	ctx := context.Background()

	acme := database.UnionAll[flow](ctx,
		paymentFlows(ctx).Where(TestPaymentCols.Account.Eq("acme")),
		refundFlows(ctx),
	).
		OrderBy(TestPaymentCols.PaidAt.As("at").Desc(), TestPaymentCols.ID.Desc()).
		Limit(2)

	total := 0
	if err := acme.Count(&total); err != nil {
		panic(err)
	}
	page := make([]flow, 0)
	if err := acme.Scan(&page); err != nil {
		panic(err)
	}
	fmt.Println("total:", total)
	for _, r := range page {
		fmt.Println(r.Kind, r.ID, r.Amount)
	}
	// Output:
	// total: 5
	// refund r2 30
	// payment p4 400
}

// A branch takes any shape a select takes: two grouped projections stack every
// account's paid total beside its refunded total. Literal stays out of GROUP
// BY, and the flow is ordered by result columns named through the payment
// model's references. Rendered:
//
//	SELECT * FROM (
//	  SELECT * FROM (SELECT 'payment' AS `kind`, `account` AS `account`, COALESCE(SUM(`amount`), 0) AS `amount`
//	                 FROM `test_payments` WHERE `test_payments`.`deleted_at` IS NULL GROUP BY `account`) AS `b0`
//	  UNION ALL
//	  SELECT * FROM (SELECT 'refund' AS `kind`, `account` AS `account`, COALESCE(SUM(`amount`), 0) AS `amount`
//	                 FROM `test_refunds` WHERE `test_refunds`.`deleted_at` IS NULL GROUP BY `account`) AS `b1`
//	) AS `u` ORDER BY `account` ASC,`amount` DESC
func ExampleUnionAll_groupedBranches() {
	seedFlowExample()
	defer cleanupFlowData()
	ctx := context.Background()

	type accountTotal struct {
		Kind    string
		Account string
		Amount  int64
	}
	paid := database.Select[*TestPayment, accountTotal](ctx,
		types.Literal("payment").As("kind"), TestPaymentCols.Account.Group(), TestPaymentCols.Amount.Sum())
	refunded := database.Select[*TestRefund, accountTotal](ctx,
		types.Literal("refund").As("kind"), TestRefundCols.Account.Group(), TestRefundCols.Amount.Sum())

	rows := make([]accountTotal, 0)
	if err := database.UnionAll[accountTotal](ctx, paid, refunded).
		OrderBy(TestPaymentCols.Account.Asc(), TestPaymentCols.Amount.Desc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Account, r.Kind, r.Amount)
	}
	// Output:
	// acme payment 700
	// acme refund 50
	// bolt payment 300
	// bolt refund 30
}
