package types

// Selector runs an analytical read over the table of M and scans the result
// rows into R. It is deliberately separate from Database[M]: an aggregate
// result is not a model row, so model hooks, association preloading and cursor
// pagination have nothing to act on and are absent here rather than present
// and inert.
//
// Scoping comes from M — the table name, the soft-delete condition and the
// dialect — so an aggregate can never read rows a List on the same model
// hides. R is an ordinary struct the caller declares; its fields bind to the
// projection aliases, and a mismatch on either side is a build error rather
// than a silently zero column. A measure that can come back NULL — AVG, MIN
// or MAX without group keys, carrying conditions, or over a nullable column —
// must bind to a pointer or sql.Null field, which is again a build error
// rather than a zero on the report.
//
// The entry point is the package-level database.Select[M, R] rather than a
// method, because a Go method cannot introduce the result type parameter.
//
// Row-level access rules are not inherited. A model's List gets its tenant or
// group scoping from the Filter service hook the controller runs;
// an aggregate is called straight from service code, so those hooks never run
// and every scoping condition has to be passed to Where explicitly. Forgetting
// one aggregates across tenants without any sign that it did.
//
// The projection is declared at the entry point and takes one of two shapes.
// A grouped projection carries aggregates: a term without a function is a
// group key, and GROUP BY is derived from those keys, so the SELECT and
// GROUP BY lists cannot disagree. A row-level projection carries window
// functions and no aggregate: every row keeps its place, a column reference
// passed directly is projected as stored, and the window functions add their
// value per row. A projection with neither an aggregate nor a window
// function is a plain read and belongs to List.
//
// A builder is a specification, not a live statement: it can be read more than
// once, and each terminal renders the spec afresh, taking only the parts that
// are meaningful to it — Scan and ScanOne render everything, Count
// ignores OrderBy, Limit and Offset because none of them changes how many
// groups exist. That is what makes the paginated-report idiom safe — Scan for
// the page, then Count for the total, off the same builder, with the
// pagination never skewing the count.
//
// Example:
//
//	type tenantTotal struct {
//	    TenantID string
//	    Total    int64
//	    // ClosedAt is nullable on Sample, so MAX over it can be NULL and
//	    // needs a field that can hold NULL.
//	    LastClosed *time.Time
//	}
//	total := SampleCols.Amount.Sum().As("total")
//	rows := make([]tenantTotal, 0)
//	err := database.Select[*Sample, tenantTotal](ctx,
//	    SampleCols.TenantID.Group(), total, SampleCols.ClosedAt.Max().As("last_closed")).
//	    Where(SampleCols.Status.Eq(StatusDone)).
//	    Having(total.Gte(1000)).
//	    OrderBy(total.Desc()).
//	    Limit(10).
//	    Scan(&rows)
type Selector[M Model, R any] interface {
	// Where restricts the rows entering the aggregation, using the same filter
	// tree as WithQuery.
	Where(filters ...Filter) Selector[M, R]
	// Having restricts the produced groups by their measures.
	Having(conditions ...TermCondition) Selector[M, R]
	// Qualify restricts the result rows by their window functions, which a
	// WHERE cannot see: the framework wraps the projection in a derived
	// table and filters that. A condition must name a window term the
	// projection declares.
	Qualify(conditions ...TermCondition) Selector[M, R]
	// OrderBy sorts the result rows by a projected column or term.
	OrderBy(orders ...Ordering) Selector[M, R]
	// Limit caps the number of result rows.
	Limit(n int) Selector[M, R]
	// Offset skips result rows, for paginating a grouped report.
	Offset(n int) Selector[M, R]

	// Scan runs the query and fills dest with one element per group.
	Scan(dest *[]R) error
	// ScanOne runs an ungrouped aggregation and fills dest with its single
	// row. It fails when the projection declares group keys.
	ScanOne(dest *R) error
	// Count reports how many groups the query produces, which is the
	// total a paginated grouped report needs. OrderBy, Limit and Offset set on
	// the builder do not apply to it.
	Count(count *int) error

	// WithDryRun builds the SQL without database I/O. An optional collector
	// receives the generated Query, Args, and RenderedSQL of the next
	// terminal operation instead of executing it.
	WithDryRun(collector ...*[]SQLStatement) Selector[M, R]
}
