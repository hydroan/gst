package types

// TermFn is the function applied to one projection term. The set is
// closed, so a projection can never carry SQL the way a free-form select
// string could: the renderer maps each constant to a fixed expression and
// rejects anything else.
type TermFn string

const (
	// FnNone marks a group key rather than a measure. A projection term
	// without an aggregate function is what the framework derives GROUP BY
	// from, so the SELECT and GROUP BY lists can never disagree.
	FnNone          TermFn = ""
	FnCount         TermFn = "COUNT"
	FnCountDistinct TermFn = "COUNT_DISTINCT"
	FnSum           TermFn = "SUM"
	FnAvg           TermFn = "AVG"
	FnMin           TermFn = "MIN"
	FnMax           TermFn = "MAX"

	// FnRowNumber, FnRank, FnDenseRank, FnLag and FnLead are the window
	// functions, only meaningful over a window, which Term.Over declares. A
	// term carrying one of them without a window is a build error, as is
	// COUNT DISTINCT over a window, which no dialect supports.
	FnRowNumber TermFn = "ROW_NUMBER"
	FnRank      TermFn = "RANK"
	FnDenseRank TermFn = "DENSE_RANK"
	FnLag       TermFn = "LAG"
	FnLead      TermFn = "LEAD"
)

// Valid reports whether the function is one this package defines. The renderer
// composes SQL from the constant, so a value from outside the set would reach
// the statement as text; the query builder rejects it instead.
func (f TermFn) Valid() bool {
	switch f {
	case FnNone, FnCount, FnCountDistinct,
		FnSum, FnAvg, FnMin, FnMax,
		FnRowNumber, FnRank, FnDenseRank, FnLag, FnLead:
		return true
	default:
		return false
	}
}

// TimeBucket is the truncation granularity of a time group key. Bucketing is
// the one place where the same intent needs a different expression per
// dialect, so the constant travels through the builder and the database layer
// renders it; callers never see a format string.
type TimeBucket string

const (
	// TimeBucketNone groups by the raw column value.
	TimeBucketNone  TimeBucket = ""
	TimeBucketHour  TimeBucket = "hour"
	TimeBucketDay   TimeBucket = "day"
	TimeBucketMonth TimeBucket = "month"
)

// Valid reports whether the bucket is one this package defines. An unknown
// bucket would otherwise fall through to the day granularity and silently
// report the wrong period.
func (b TimeBucket) Valid() bool {
	switch b {
	case TimeBucketNone, TimeBucketHour, TimeBucketDay, TimeBucketMonth:
		return true
	default:
		return false
	}
}

// Term is one term of a projection: a group key when Fn is FnNone, a plain
// column when Plain is also set, a measure otherwise, and a window function
// when Window is set.
//
// Terms are built through the column references: the generated Cols vars,
// or references minted with NewColumn and its siblings by code that has no
// generated file, such as framework module sources. A generated reference
// cannot express a function its column type does not support, because it
// does not carry the method; a minted reference names whatever type its
// author chose, so the column and its type are checked against the model
// schema when the query is built.
//
// A term never holds SQL. Column names are quoted by the database layer,
// values bind as statement parameters, and Fn and Bucket come from closed sets.
type Term struct {
	// Fn is the aggregate or window function, or FnNone for a group key or
	// a plain column.
	Fn TermFn
	// Plain marks a column projected as it is stored, which is what a column
	// reference selects as when it is passed to Select directly. It belongs
	// to a row-level projection, one carrying window functions and no
	// aggregate; next to an aggregate it is a build error, because there
	// every column has to be a group key or be aggregated.
	Plain bool
	// Window makes the function a window function, evaluated over the rows
	// the window names instead of collapsing them; see Over.
	Window *Window
	// Table is the table the column belongs to, carried over from the column
	// reference. It is empty only on COUNT(*), which names no column.
	Table string
	// Column is the snake case column name. It is empty only for COUNT(*).
	Column string
	// Bucket truncates a time group key. It is only meaningful when Fn is
	// FnNone and the column is a time column.
	Bucket TimeBucket
	// Conditions restrict a measure to the rows matching them, rendering as a
	// CASE expression inside the aggregate call. They reuse the query filter
	// tree, so conditional aggregation needs no predicate language of its own.
	Conditions []Filter
	// Alias names the term in the SELECT list and binds it to a field of the
	// result row. An empty alias defaults to the column name.
	Alias string
}

// IsMeasure reports whether the term applies a function rather than naming a
// column, a window function included.
func (t Term) IsMeasure() bool { return t.Fn != FnNone }

// IsWindowed reports whether the term is evaluated over a window.
func (t Term) IsWindowed() bool { return t.Window != nil }

// IsGroupKey reports whether the term is a group key: a column or time bucket
// projected next to aggregates, which the framework derives GROUP BY from.
func (t Term) IsGroupKey() bool { return t.Fn == FnNone && !t.Plain }

// IsPlain reports whether the term is a column projected as it is stored.
func (t Term) IsPlain() bool { return t.Fn == FnNone && t.Plain }

// As renames the term in the SELECT list.
//
// It is optional. Every term already carries a default alias — the column name
// for a column term, "count" for COUNT(*) — so a projection whose result row
// fields are named after the columns needs no As at all:
//
//	database.Select[*Sample, row](ctx, SampleCols.TenantID.Group(), SampleCols.Amount.Sum())
//	// scans into struct{ TenantID string; Amount int64 }
//
// Reach for As in the two cases the default cannot cover: when the result row
// field is named differently from the column, and when one projection carries
// two terms over the same column, whose default aliases would collide.
//
// The alias belongs to the result contract rather than to the column, which is
// why it is applied here instead of being a parameter of the constructors.
func (t Term) As(alias string) Term {
	t.Alias = alias
	return t
}

// Where restricts a measure to the rows matching filters, which is how a
// report projects several measures over different subsets in a single scan:
//
//	Count().Where(SampleCols.Status.Eq(StatusFailed)).As("failed")
//	// COUNT(CASE WHEN `status` = ? THEN 1 END) AS `failed`
//
// The filters are the ordinary query filters, including nested groups, so the
// same fail-closed rules and the same renderer apply.
func (t Term) Where(filters ...Filter) Term {
	t.Conditions = append(append([]Filter(nil), t.Conditions...), filters...)
	return t
}

// DefaultCountAlias is the alias COUNT(*) projects under when the caller does
// not rename it. A column term defaults to its column name, but COUNT(*) names
// no column, so without a default of its own it would be the one term that
// always had to be renamed.
const DefaultCountAlias = "count"

// Count counts rows: COUNT(*). It counts a row even when every column is NULL,
// which is what a plain row count means; use a column reference's Count for
// COUNT(column), which skips NULLs.
//
// It projects as "count" unless renamed with As.
func Count() Term {
	return Term{Fn: FnCount, Alias: DefaultCountAlias}
}

// Over evaluates the term over a window instead of collapsing the rows it
// reads: every row keeps its place and gains the function's value computed
// over the rows the window names. It applies to the aggregate functions and
// to the window functions; a group key, a time bucket and COUNT DISTINCT
// cannot be windowed and fail when the query is built.
//
// With an ordered window the aggregate functions accumulate: SUM becomes a
// running total, COUNT a running count, and so on, always over the rows from
// the partition's first up to the current one — the frame is fixed to that,
// so two rows sorting equal never fold into one step the way the SQL default
// frame would fold them.
//
//	SampleCols.Amount.Sum().Over(PartitionBy(SampleCols.TenantID).OrderBy(SampleCols.CreatedAt.Asc()))
//	// COALESCE(SUM(`amount`) OVER (PARTITION BY `tenant_id` ORDER BY `created_at` ASC, `id` ASC
//	//   ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW), 0)
func (t Term) Over(window Window) Term {
	t.Window = &window
	return t
}

// The window functions below carry no column and, like Count, project under
// a default alias until renamed with As. They only exist over a window whose
// OrderBy is set: without an order there is no first row to number.

// RowNumber numbers the rows of each partition from 1 in the window's order,
// with no ties: two rows sorting equal still get consecutive numbers, in a
// stable order the framework completes with the primary key.
func RowNumber() Term { return Term{Fn: FnRowNumber, Alias: "row_number"} }

// Rank ranks the rows of each partition in the window's order. Rows sorting
// equal share a rank and the next rank skips past them: 1, 2, 2, 4.
func Rank() Term { return Term{Fn: FnRank, Alias: "rank"} }

// DenseRank ranks like Rank without skipping: 1, 2, 2, 3.
func DenseRank() Term { return Term{Fn: FnDenseRank, Alias: "dense_rank"} }

// Expr is what a projection selects and a window partitions by: a column
// reference, projected as it is stored, or a Term. The set is closed to this
// package, so a projection can never carry SQL text.
type Expr interface {
	exprTerm() Term
}

func (t Term) exprTerm() Term { return t }

// TermOf returns the term an expression selects as: a term unchanged, a column
// reference as the plain projection of that column.
//
// It is the one way a renderer outside this package reads an Expr, and it
// exists for two reasons. The interface is sealed through an unexported
// method so the set of expressions stays closed, which also puts that method
// out of reach of the database package. And Column is generic, so a type
// switch there cannot enumerate its instantiations the way it enumerates the
// two Ordering types. It is an accessor, not a constructor: removing it would
// leave the database layer no way to turn a projection into terms.
func TermOf(expr Expr) Term { return expr.exprTerm() }

// CompareOp is a comparison applied to a projected term. Only the six
// orderings exist: the pattern and set operators of FilterOp have no meaning
// over a measure or a window function.
type CompareOp string

const (
	CompareEq  CompareOp = "eq"
	CompareNe  CompareOp = "ne"
	CompareGt  CompareOp = "gt"
	CompareGte CompareOp = "gte"
	CompareLt  CompareOp = "lt"
	CompareLte CompareOp = "lte"
)

// Valid reports whether the comparison is one this package defines. An unknown
// operator would otherwise fall through to equality and silently filter by the
// wrong comparison.
func (o CompareOp) Valid() bool {
	switch o {
	case CompareEq, CompareNe, CompareGt, CompareGte, CompareLt, CompareLte:
		return true
	default:
		return false
	}
}

// TermCondition is one condition on a projected term: a Having condition on a
// measure, or a Qualify condition on a window function. It carries the term
// itself rather than an alias string, which has two consequences: a condition
// can never name a term the projection did not declare, and the renderer can
// emit the full expression instead of the alias, which HAVING requires because
// PostgreSQL does not accept an output alias there.
type TermCondition struct {
	Term  Term
	Op    CompareOp
	Value any
}

// Eq, Ne, Gt, Gte, Lt and Lte build a Having or Qualify condition on the
// term. The value type is checked when the query is built, because a
// function's value type follows the function rather than its column: COUNT
// always yields an integer, AVG a float, and SUM widens.
func (t Term) Eq(value any) TermCondition { return TermCondition{Term: t, Op: CompareEq, Value: value} }

func (t Term) Ne(value any) TermCondition { return TermCondition{Term: t, Op: CompareNe, Value: value} }

func (t Term) Gt(value any) TermCondition { return TermCondition{Term: t, Op: CompareGt, Value: value} }

func (t Term) Gte(value any) TermCondition {
	return TermCondition{Term: t, Op: CompareGte, Value: value}
}

func (t Term) Lt(value any) TermCondition { return TermCondition{Term: t, Op: CompareLt, Value: value} }

func (t Term) Lte(value any) TermCondition {
	return TermCondition{Term: t, Op: CompareLte, Value: value}
}

// TermOrder is one ORDER BY term of a select or of a window. Unlike Order it
// sorts by a projection term, which is what a TopN report ranks by.
type TermOrder struct {
	Term      Term
	Direction OrderDirection
}

// Asc and Desc sort the rows by this term. In the select's ORDER BY the term
// renders as its alias, which every supported dialect accepts there; inside a
// window it renders as its full expression, the only form OVER accepts.
func (t Term) Asc() TermOrder {
	return TermOrder{Term: t, Direction: OrderAsc}
}

func (t Term) Desc() TermOrder {
	return TermOrder{Term: t, Direction: OrderDesc}
}

func (TermOrder) sealedOrdering() {}
