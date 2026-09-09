package types

// SelectBranch is a select in the role of a branch of a union: every Selector
// is one, with its model type erased, so that selects over different models
// stack into one result as long as they scan into the same row type. The
// role is what UnionAll takes. Only the selects the database layer builds can
// fill it; UnionAll fails when handed anything else, a union among them.
type SelectBranch[R any] interface {
	Scan(dest *[]R) error
	Count(count *int) error
}

// Union stacks the rows of several selects into one result: UNION ALL, the
// one set operation the framework offers. UNION proper would fold two rows
// that happen to be equal — two payments of the same amount on the same day
// — into one, which no report wants; INTERSECT and EXCEPT are the semi joins
// FilterExists and FilterNotExists already express.
//
// The branches are ordinary selects, and the alignment a hand-written UNION
// gets wrong by position is done by name: every branch scans into R, so its
// aliases are checked against R's fields, and the framework renders every
// branch's SELECT list in R's field order. A column spelled differently in
// one model is aligned with Column.As; a constant that tells the rows apart
// is projected with Literal:
//
//	type flow struct {
//	    Kind    string
//	    ID      string
//	    Amount  int64
//	    At      time.Time
//	}
//	payments := database.Select[*Payment, flow](ctx,
//	    types.Literal("payment").As("kind"), PaymentCols.ID, PaymentCols.Amount, PaymentCols.PaidAt.As("at")).
//	    Where(PaymentCols.TenantID.Eq(tenant))
//	refunds := database.Select[*Refund, flow](ctx,
//	    types.Literal("refund").As("kind"), RefundCols.ID, RefundCols.Amount, RefundCols.SettledAt.As("at")).
//	    Where(RefundCols.TenantID.Eq(tenant))
//	feed := database.UnionAll[flow](ctx, payments, refunds).
//	    OrderBy(PaymentCols.PaidAt.As("at").Desc(), PaymentCols.ID.Desc()).
//	    Page(3, 20)
//	err := feed.Scan(&rows)
//	err = feed.Count(&total)
//
// Rendered, with the ordering and the page pushed into every branch — the
// bound limit is the rows before the page plus the page, 60 here — so each
// branch reads its first
// sixty rows by its own index and the union sorts a hundred and twenty rows at
// most:
//
//	SELECT * FROM (
//	  SELECT * FROM (SELECT 'payment' AS `kind`, `id` AS `id`, `amount` AS `amount`, `paid_at` AS `at`
//	                 FROM `payments` WHERE `tenant_id` = ? AND `payments`.`deleted_at` IS NULL
//	                 ORDER BY `at` DESC,`id` DESC LIMIT ?) AS `b0`
//	  UNION ALL
//	  SELECT * FROM (SELECT 'refund' AS `kind`, `id` AS `id`, `amount` AS `amount`, `settled_at` AS `at`
//	                 FROM `refunds` WHERE `tenant_id` = ? AND `refunds`.`deleted_at` IS NULL
//	                 ORDER BY `at` DESC,`id` DESC LIMIT ?) AS `b1`
//	) AS `u` ORDER BY `at` DESC,`id` DESC LIMIT ? OFFSET ?
//
// A branch keeps its own Where, Having and Qualify, and may be a plain
// projection of columns, which a select on its own rejects: stacked, the rows
// are a report rather than a List. OrderBy, Limit and Page belong to the
// union and are a build error on a branch. The union has no Where: a
// condition belongs to the branch whose index serves it.
//
// Count adds up the counts of the branches and materializes no row. Like a
// select's Count it ignores OrderBy, Limit and Page.
//
// Every dialect renders the same statement; on ClickHouse the union is opened
// with UnionAllOn, on the instance the branches were opened on.
type Union[R any] interface {
	// OrderBy sorts the stacked rows by their columns. An ordering names a
	// column of the result row, through a column reference of that name or
	// through a term a branch projects under it. The name alone decides: the
	// union reads no table of its own, so the table a reference carries is
	// not checked here.
	OrderBy(orders ...Ordering) Union[R]
	// Limit caps the number of stacked rows, read from the first; it drops
	// the skip of a Page set before it.
	Limit(n int) Union[R]
	// Page keeps one page of the stacked rows, as Page does on a select.
	Page(page, size int) Union[R]

	// Scan runs the union and fills dest with the stacked rows.
	Scan(dest *[]R) error
	// Count reports how many rows the branches produce together.
	Count(count *int) error

	// WithDryRun builds the SQL without database I/O; see
	// Selector.WithDryRun. A dry run set on a branch is the branch's own:
	// the union's terminal runs the branch for real and consumes it,
	// whether or not the union builds.
	WithDryRun(collector ...*[]SQLStatement) Union[R]
}
