package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// TermFn is the function applied to one projection term. The set is
// closed, so a projection can never carry SQL the way a free-form select
// string could: the renderer maps each constant to a fixed expression and
// rejects anything else.
type TermFn = itypes.TermFn

const (
	// FnNone marks a group key rather than a measure. A projection term
	// without an aggregate function is what the framework derives GROUP BY
	// from, so the SELECT and GROUP BY lists can never disagree.
	FnNone          = itypes.FnNone
	FnCount         = itypes.FnCount
	FnCountDistinct = itypes.FnCountDistinct
	FnSum           = itypes.FnSum
	FnAvg           = itypes.FnAvg
	FnMin           = itypes.FnMin
	FnMax           = itypes.FnMax
	// FnRowNumber, FnRank, FnDenseRank, FnLag and FnLead are the window
	// functions, only meaningful over a window, which Term.Over declares. A
	// term carrying one of them without a window is a build error, as is
	// COUNT DISTINCT over a window, which no dialect supports.
	FnRowNumber = itypes.FnRowNumber
	FnRank      = itypes.FnRank
	FnDenseRank = itypes.FnDenseRank
	FnLag       = itypes.FnLag
	FnLead      = itypes.FnLead
	// FnLiteral marks a constant projected as a column, which Literal
	// builds. Like FnNone it is a kind rather than a function: the term
	// carries its value in Literal and names no column.
	FnLiteral = itypes.FnLiteral
)

// TimeBucket is the truncation granularity of a time group key. Bucketing is
// the one place where the same intent needs a different expression per
// dialect, so the constant travels through the builder and the database layer
// renders it; callers never see a format string.
type TimeBucket = itypes.TimeBucket

const (
	// TimeBucketNone groups by the raw column value.
	TimeBucketNone  = itypes.TimeBucketNone
	TimeBucketHour  = itypes.TimeBucketHour
	TimeBucketDay   = itypes.TimeBucketDay
	TimeBucketMonth = itypes.TimeBucketMonth
)

// Term is one term of a projection: a group key when Fn is FnNone, a plain
// column when Plain is also set, a constant when Fn is FnLiteral, a measure
// otherwise, and a window function when Window is set.
type Term = itypes.Term

// DefaultCountAlias is the alias COUNT(*) projects under when the caller does
// not rename it. A column term defaults to its column name, but COUNT(*) names
// no column, so without a default of its own it would be the one term that
// always had to be renamed.
const DefaultCountAlias = itypes.DefaultCountAlias

// Count counts rows: COUNT(*). It counts a row even when every column is NULL,
// which is what a plain row count means; use a column reference's Count for
// COUNT(column), which skips NULLs.
func Count() Term {
	return itypes.Count()
}

// RowNumber numbers the rows of each partition from 1 in the window's order,
// with no ties: two rows sorting equal still get consecutive numbers, in a
// stable order the framework completes with the primary key. Like Rank and
// DenseRank it only exists over a window whose OrderBy is set, which Over
// declares: without an order there is no first row to number.
func RowNumber() Term {
	return itypes.RowNumber()
}

// Rank ranks the rows of each partition in the window's order. Rows sorting
// equal share a rank and the next rank skips past them: 1, 2, 2, 4.
func Rank() Term {
	return itypes.Rank()
}

// DenseRank ranks like Rank without skipping: 1, 2, 2, 3.
func DenseRank() Term {
	return itypes.DenseRank()
}

// Literal projects a constant, which is how the branches of a union tell
// their rows apart.
func Literal(value string) Term {
	return itypes.Literal(value)
}

// Expr is what a projection selects and a window partitions by: a column
// reference, projected as it is stored, or a Term. The set is closed to the
// framework, so a projection can never carry SQL text.
type Expr = itypes.Expr

// TermOf returns the term an expression selects as: a term unchanged, a column
// reference as the plain projection of that column.
func TermOf(expr Expr) Term {
	return itypes.TermOf(expr)
}

// CompareOp is a comparison applied to a projected term. Only the six
// orderings exist: the pattern and set operators of FilterOp have no meaning
// over a measure or a window function.
type CompareOp = itypes.CompareOp

const (
	CompareEq  = itypes.CompareEq
	CompareNe  = itypes.CompareNe
	CompareGt  = itypes.CompareGt
	CompareGte = itypes.CompareGte
	CompareLt  = itypes.CompareLt
	CompareLte = itypes.CompareLte
)

// TermCondition is one condition on a projected term: a Having condition on a
// measure, or a Qualify condition on a window function. It carries the term
// itself rather than an alias string, which has two consequences: a condition
// can never name a term the projection did not declare, and the renderer can
// emit the full expression instead of the alias, which HAVING requires because
// PostgreSQL does not accept an output alias there.
type TermCondition = itypes.TermCondition

// TermOrder is one ORDER BY term of a select or of a window. Unlike Order it
// sorts by a projection term, which is what a TopN report ranks by.
type TermOrder = itypes.TermOrder
