package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// Term is one term of a projection: a group key, a plain column, a constant, a
// measure, or a window function. Column references build it, as do Count, the
// ranking functions and Literal below.
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
