package types

// Selector runs an analytical read over the table of M and scans the result
// rows into R. It is deliberately separate from Database[M]: a projected row
// is not a model row, so model hooks, association preloading and cursor
// pagination have nothing to act on and are absent here rather than present
// and inert.
//
// Scoping comes from M — the table name, the soft-delete condition and the
// dialect — so a select can never read rows a List on the same model hides.
// R is an ordinary struct the caller declares; its fields bind to the
// projection aliases, and a mismatch on either side is a build error rather
// than a silently zero column. A term that can come back NULL — AVG, MIN or
// MAX without group keys, carrying conditions, or over a nullable column; LAG
// and LEAD; a plain column that is nullable — must bind to a pointer or
// sql.Null field, which is again a build error rather than a zero on the
// report.
//
// The entry point is the package-level database.Select[M, R] rather than a
// method, because a Go method cannot introduce the result type parameter.
//
// The rules a model declares on its rows travel with it: soft deletion and
// the tenant scope a model embeds apply to a select as they do to List, and
// a joined model carries them in its ON. What a select does not inherit is
// the scoping a service adds in the Filter hook the controller runs for
// List; a select is called straight from service code, so those hooks never
// run and every such condition has to be passed to Where explicitly.
// Forgetting one reads across the boundary the hook draws without any sign
// that it did.
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
// A select joins other models on a unique key with Join: the joined model's
// columns then project, filter, group, partition and order like the queried
// model's own, and the projection may be plain columns alone, which List
// cannot read across tables; see JoinSource for the rules.
//
// A Selector is also a SelectBranch: UnionAll stacks several of them, over
// different models, into one result. In that role a plain projection of
// columns is allowed, and OrderBy, Limit and Offset belong to the union; see
// Union.
//
// A builder is a specification, not a live statement: it can be read more than
// once, and each terminal renders the spec afresh, taking only the parts that
// are meaningful to it — Scan and ScanOne render everything, Count
// ignores OrderBy, Limit and Offset because none of them changes how many
// rows exist. That is what makes the paginated-report idiom safe — Scan for
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
	// Where restricts the rows entering the projection, using the same filter
	// tree as WithQuery.
	Where(filters ...Filter) Selector[M, R]
	// Join adds the sources the select joins: models joined on a unique key,
	// built by Join and LeftJoin, and grouped selects joined on their group
	// keys as derived tables, built by JoinSelect and LeftJoinSelect; see
	// JoinSource. A joined model's columns are referenced through its own
	// Cols everywhere in the select, a joined select's terms by passing the
	// very terms it projects.
	Join(sources ...JoinSource) Selector[M, R]
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
	// Offset skips result rows, for paginating a report.
	Offset(n int) Selector[M, R]

	// Scan runs the query and fills dest with one element per result row: a
	// group of a grouped projection, a row of a row-level one.
	Scan(dest *[]R) error
	// ScanOne runs a projection of measures alone, which is one row by
	// definition, and fills dest with it. A group key, a plain column, a
	// window function or a joined select's term makes the read grouped or
	// row-level and fails it, as do Having, Limit and Offset, which could
	// only turn the one row into none.
	ScanOne(dest *R) error
	// Count reports how many rows the query produces after Having and
	// Qualify — the groups of a grouped projection, the rows of a row-level
	// one — which is the total a paginated report needs. OrderBy, Limit and
	// Offset set on the builder do not apply to it.
	Count(count *int) error

	// WithDryRun builds the SQL without database I/O. An optional collector
	// receives the generated Query, Args, and RenderedSQL of the next
	// terminal operation instead of executing it; that terminal consumes the
	// option, so the builder read again executes. The terminal writes
	// nothing: Scan leaves dest as it was and Count leaves count untouched.
	// A builder used as a union member or a joined select is run by the
	// enclosing query's terminal, for real, which consumes the option
	// whether or not the query builds.
	WithDryRun(collector ...*[]SQLStatement) Selector[M, R]
}
