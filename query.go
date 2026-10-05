package gst

import "github.com/hydroan/gst/internal/types"

// Filter is one field-level filter to apply as an AND condition: the column it
// compares, the table that column belongs to, the operator and the value. Its
// fields are unexported, so a filter comes from the generated column
// references, a reference minted for a type parameter, the grouping, subquery
// and constant constructors below, or the framework's URL parsing. Table,
// Column, Op and Value read it back; Split, Values, ExcludedValues and Bounds
// on a column reference read one column's filters converted to the column's
// type. A filter carrying another table is applied to that table when the
// query joins it and fails closed otherwise, and the value is always bound as
// a statement parameter.
type Filter = types.Filter

// FilterFalse matches nothing. It is the condition a permission hook returns
// when the caller may see no row at all. Unlike an empty filter list it is a
// real condition, so it disables the empty-query safety check; unlike a
// filter the renderer cannot apply it is deliberate, so nothing is logged. It
// renders as 1 = 0 on every dialect and composes like any other filter,
// inside groups, subqueries and conditional measures included.
func FilterFalse() Filter {
	return types.FilterFalse()
}

// FilterOr groups filters that are OR-combined with each other. The group as a
// whole stays AND-combined with every other condition of the query, so a
// mandatory condition such as tenant scoping can never be absorbed into the
// alternatives.
func FilterOr(filters ...Filter) Filter {
	return types.FilterOr(filters...)
}

// FilterAnd groups filters that are AND-combined with each other. Filters are
// already AND-combined at the top level, so the group exists to nest an AND
// inside an OR group.
func FilterAnd(filters ...Filter) Filter {
	return types.FilterAnd(filters...)
}

// FilterOp is a field-level filter operator: the comparison a Filter applies,
// which Filter.Op reads back. Operators never widen a query: unknown values
// are rejected during parsing, and the database layer fails closed on
// conditions it does not recognize.
type FilterOp = types.FilterOp

// URL-exposed operators, which a request spells as "field[op]=value".
const (
	FilterOpEq         = types.FilterOpEq         // equal: column = value
	FilterOpNe         = types.FilterOpNe         // not equal: column <> value
	FilterOpGt         = types.FilterOpGt         // greater than: column > value
	FilterOpGte        = types.FilterOpGte        // greater than or equal: column >= value
	FilterOpLt         = types.FilterOpLt         // less than: column < value
	FilterOpLte        = types.FilterOpLte        // less than or equal: column <= value
	FilterOpIn         = types.FilterOpIn         // set membership: column IN (comma-separated values)
	FilterOpNotIn      = types.FilterOpNotIn      // set exclusion: column NOT IN (comma-separated values)
	FilterOpLike       = types.FilterOpLike       // substring match: column LIKE %value%
	FilterOpNotLike    = types.FilterOpNotLike    // substring exclusion: column NOT LIKE %value%
	FilterOpStartsWith = types.FilterOpStartsWith // prefix match: column LIKE value% (can use an index)
	FilterOpEndsWith   = types.FilterOpEndsWith   // suffix match: column LIKE %value
	FilterOpIsNull     = types.FilterOpIsNull     // null check: value true means IS NULL, false means IS NOT NULL
)

// Service-only operators, which service code builds and no request can spell.
const (
	FilterOpRegex        = types.FilterOpRegex        // regular expression match: column REGEXP value (dialect-aware)
	FilterOpNotRegex     = types.FilterOpNotRegex     // regular expression exclusion: NOT (column REGEXP value)
	FilterOpJSONContains = types.FilterOpJSONContains // JSON array membership: value is a member of the JSON array column
	FilterOpOr           = types.FilterOpOr           // group: the []Filter value is OR-combined, the group itself AND-combined
	FilterOpAnd          = types.FilterOpAnd          // group: the []Filter value is AND-combined, for nesting inside an OR group
	FilterOpExists       = types.FilterOpExists       // correlated subquery: EXISTS or NOT EXISTS over a related model
	FilterOpEqCol        = types.FilterOpEqCol        // column equals another column: the enclosing query's inside a subquery, a table read beside it inside a join
	FilterOpFalse        = types.FilterOpFalse        // constant predicate: matches nothing, see FilterFalse
)

// FilterExists matches rows of the queried model that have at least one
// related row in C satisfying filters. EqCol predicates tie the related
// rows to the queried row, one per column pair, next to the ordinary
// conditions narrowing them.
func FilterExists[C Model](filters ...Filter) Filter {
	return types.FilterExists[C](filters...)
}

// FilterNotExists matches rows that have no related row in C satisfying
// filters. Note that it is not the negation of a filtered FilterExists over the
// same rows: a row whose related rows all fail filters matches, and so does a
// row with no related rows at all. A subquery without any EqCol predicate
// fails closed here as well: negating "match nothing" would otherwise widen
// into "match everything".
func FilterNotExists[C Model](filters ...Filter) Filter {
	return types.FilterNotExists[C](filters...)
}

// Order is one ORDER BY term: a column and the direction to sort it by. Its
// fields are unexported, so an order comes from a generated column reference,
// a reference minted for a type parameter, or the framework's URL parsing;
// Table, Column and Descending read it back, and SortsBy tells whether it
// sorts by a given column. Table is filled in by a column reference and left
// empty by URL parsing. The chain's reads and a select check it: a select that
// joins tells two tables' columns of one name apart by it, and a chain refuses
// an order of another model. A union orders its result columns by name and
// reads no table, and WithExpand orders the associated table, so neither
// checks it. An Order with an empty column is skipped rather than rendered.
type Order = types.Order

// Ordering is what the OrderBy methods of a select, a window and a union
// accept: an Order sorting by a column reference, or a TermOrder sorting by a
// projection term. The set is closed, so an ordering can never carry SQL the
// way a free-form string could.
type Ordering = types.Ordering

// Cursor is where a cursor-paginated read starts and which way it goes: the
// feed's stable ordering, the boundary row, and whether the read travels along
// that ordering or back down it. Its fields are unexported, so a cursor comes
// from CursorForward, CursorBackward or the framework's URL parsing; Order,
// Value and Backward read it back.
type Cursor = types.Cursor

// CursorForward pages along order, starting just past value.
func CursorForward(order Order, value string) Cursor {
	return types.CursorForward(order, value)
}

// CursorBackward pages against order, starting just before value.
func CursorBackward(order Order, value string) Cursor {
	return types.CursorBackward(order, value)
}

// Assignment is one column-value write, the unit UpdateByID accepts. Service
// code builds assignments through the generated column references
// (SampleCols.Status.Set(v)), whose typed front end stops a wrong-typed value
// or a misspelled column at compile time; generic code assigns through a
// reference minted for its type parameter. Its fields are unexported, so those
// references are the only way to build one; Table, Column and Value read it
// back.
type Assignment = types.Assignment

// Term is one term of a projection: a group key, a plain column, a constant, a
// measure, or a window function. Column references build it, as do Count, the
// ranking functions and Literal below.
type Term = types.Term

// DefaultCountAlias is the alias COUNT(*) projects under when the caller does
// not rename it. A column term defaults to its column name, but COUNT(*) names
// no column, so without a default of its own it would be the one term that
// always had to be renamed.
const DefaultCountAlias = types.DefaultCountAlias

// Count counts rows: COUNT(*). It counts a row even when every column is NULL,
// which is what a plain row count means; use a column reference's Count for
// COUNT(column), which skips NULLs.
func Count() Term {
	return types.Count()
}

// RowNumber numbers the rows of each partition from 1 in the window's order,
// with no ties: two rows sorting equal still get consecutive numbers, in a
// stable order the framework completes with the primary key. Like Rank and
// DenseRank it only exists over a window whose OrderBy is set, which Over
// declares: without an order there is no first row to number.
func RowNumber() Term {
	return types.RowNumber()
}

// Rank ranks the rows of each partition in the window's order. Rows sorting
// equal share a rank and the next rank skips past them: 1, 2, 2, 4.
func Rank() Term {
	return types.Rank()
}

// DenseRank ranks like Rank without skipping: 1, 2, 2, 3.
func DenseRank() Term {
	return types.DenseRank()
}

// Literal projects a constant, which is how the branches of a union tell
// their rows apart.
func Literal(value string) Term {
	return types.Literal(value)
}

// Expr is what a projection selects and a window partitions by: a column
// reference, projected as it is stored, or a Term. The set is closed to the
// framework, so a projection can never carry SQL text.
type Expr = types.Expr

// TermCondition is one condition on a projected term: a Having condition on a
// measure, or a Qualify condition on a window function. It carries the term
// itself rather than an alias string, which has two consequences: a condition
// can never name a term the projection did not declare, and the renderer can
// emit the full expression instead of the alias, which HAVING requires because
// PostgreSQL does not accept an output alias there.
type TermCondition = types.TermCondition

// TermOrder is one ORDER BY term of a select or of a window. Unlike Order it
// sorts by a projection term, which is what a TopN report ranks by.
type TermOrder = types.TermOrder

// Window names the rows a window function reads for each row: PartitionBy
// splits the rows into partitions, and OrderBy orders each partition, which is
// what gives a running total its direction and a row number its sequence.
type Window = types.Window

// PartitionBy opens a window partitioned by keys. Without keys the whole
// result is one partition, which is what a ranking over every row wants, and
// the window is the one OrderBy opens: the two spellings build the same
// value, so a term declared with one is found by the other.
func PartitionBy(keys ...Expr) Window {
	return types.PartitionBy(keys...)
}

// OrderBy returns a Window with no partition and the given orders: the
// window a ranking across every row reads. It is the short spelling of
// PartitionBy().OrderBy(orders...), the two building the same window; a
// window with keys starts from PartitionBy. It orders the window, not the
// result: the result is ordered by the Selector's OrderBy.
func OrderBy(orders ...Ordering) Window {
	return types.OrderBy(orders...)
}

// JoinSource is a source a select joins to its model. The set is closed to
// the framework: Join and LeftJoin join a model on a unique key, JoinSelect
// and LeftJoinSelect join a grouped select on its group keys.
type JoinSource = types.JoinSource

// Join joins model C on a unique key: JOIN, keeping only the rows of the
// query that match a row of C. The predicates are the ON condition.
func Join[C Model](on ...Filter) JoinSource {
	return types.Join[C](on...)
}

// LeftJoin joins model C on a unique key, keeping the rows of the query that
// match no row of C with the joined columns NULL: LEFT JOIN. The rules match
// Join; the result fields the joined columns bind to must hold NULL.
func LeftJoin[C Model](on ...Filter) JoinSource {
	return types.LeftJoin[C](on...)
}

// JoinSelect joins a grouped select as a derived table, JOIN (SELECT ...) AS
// jN ON ..., keeping only the rows of the query that match one of its
// groups. This is how a one-to-many relation is read beside its one side:
// the many side is grouped by the key first, so every key has one row, and
// that row is joined.
func JoinSelect[R any](sub SelectBranch[R], on ...Filter) JoinSource {
	return types.JoinSelect[R](sub, on...)
}

// LeftJoinSelect joins a grouped select as a derived table, keeping the rows
// of the query that match none of its groups with the select's terms NULL:
// LEFT JOIN. The rules match JoinSelect; the result fields the select's terms
// bind to must hold NULL.
func LeftJoinSelect[R any](sub SelectBranch[R], on ...Filter) JoinSource {
	return types.LeftJoinSelect[R](sub, on...)
}

// SelectBranch is a select in the role of a branch of a union: every Selector
// is one, with its model type erased, so that selects over different models
// stack into one result as long as they scan into the same row type. The
// role is what UnionAll takes. Only the selects the database layer builds can
// fill it; UnionAll fails when handed anything else, a union among them.
type SelectBranch[R any] = types.SelectBranch[R]

// Union stacks the rows of several selects into one result: UNION ALL, the
// one set operation the framework offers. UNION proper would fold two rows
// that happen to be equal — two payments of the same amount on the same day
// — into one, which no report wants; INTERSECT and EXCEPT are the semi joins
// FilterExists and FilterNotExists already express.
type Union[R any] = types.Union[R]
