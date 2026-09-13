package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// FilterOp is a field-level filter operator: the comparison a Filter applies,
// which Filter.Op reads back. Operators never widen a query: unknown values
// are rejected during parsing, and the database layer fails closed on
// conditions it does not recognize.
type FilterOp = itypes.FilterOp

// URL-exposed operators, which a request spells as "field[op]=value".
const (
	FilterOpEq         = itypes.FilterOpEq         // equal: column = value
	FilterOpNe         = itypes.FilterOpNe         // not equal: column <> value
	FilterOpGt         = itypes.FilterOpGt         // greater than: column > value
	FilterOpGte        = itypes.FilterOpGte        // greater than or equal: column >= value
	FilterOpLt         = itypes.FilterOpLt         // less than: column < value
	FilterOpLte        = itypes.FilterOpLte        // less than or equal: column <= value
	FilterOpIn         = itypes.FilterOpIn         // set membership: column IN (comma-separated values)
	FilterOpNotIn      = itypes.FilterOpNotIn      // set exclusion: column NOT IN (comma-separated values)
	FilterOpLike       = itypes.FilterOpLike       // substring match: column LIKE %value%
	FilterOpNotLike    = itypes.FilterOpNotLike    // substring exclusion: column NOT LIKE %value%
	FilterOpStartsWith = itypes.FilterOpStartsWith // prefix match: column LIKE value% (can use an index)
	FilterOpEndsWith   = itypes.FilterOpEndsWith   // suffix match: column LIKE %value
	FilterOpIsNull     = itypes.FilterOpIsNull     // null check: value true means IS NULL, false means IS NOT NULL
)

// Service-only operators, which service code builds and no request can spell.
const (
	FilterOpRegex        = itypes.FilterOpRegex        // regular expression match: column REGEXP value (dialect-aware)
	FilterOpNotRegex     = itypes.FilterOpNotRegex     // regular expression exclusion: NOT (column REGEXP value)
	FilterOpJSONContains = itypes.FilterOpJSONContains // JSON array membership: value is a member of the JSON array column
	FilterOpOr           = itypes.FilterOpOr           // group: the []Filter value is OR-combined, the group itself AND-combined
	FilterOpAnd          = itypes.FilterOpAnd          // group: the []Filter value is AND-combined, for nesting inside an OR group
	FilterOpExists       = itypes.FilterOpExists       // correlated subquery: EXISTS or NOT EXISTS over a related model
	FilterOpEqCol        = itypes.FilterOpEqCol        // column equals another column: the enclosing query's inside a subquery, a table read beside it inside a join
	FilterOpFalse        = itypes.FilterOpFalse        // constant predicate: matches nothing, see FilterFalse
)

// Filter is one field-level filter to apply as an AND condition: the column it
// compares, the table that column belongs to, the operator and the value. Its
// fields are unexported, so a filter comes from the generated column
// references, a reference minted for a type parameter, the grouping, subquery
// and constant constructors below, or the framework's URL parsing. Table,
// Column, Op and Value read it back; Split, Values and Bounds on a column
// reference read one column's filters converted to the column's type. A filter
// carrying another table is applied to that table when the query joins it and
// fails closed otherwise, and the value is always bound as a statement
// parameter.
type Filter = itypes.Filter

// FilterFalse matches nothing. It is the condition a permission hook returns
// when the caller may see no row at all. Unlike an empty filter list it is a
// real condition, so it disables the empty-query safety check; unlike a
// filter the renderer cannot apply it is deliberate, so nothing is logged. It
// renders as 1 = 0 on every dialect and composes like any other filter,
// inside groups, subqueries and conditional measures included.
func FilterFalse() Filter {
	return itypes.FilterFalse()
}

// FilterOr groups filters that are OR-combined with each other. The group as a
// whole stays AND-combined with every other condition of the query, so a
// mandatory condition such as tenant scoping can never be absorbed into the
// alternatives.
func FilterOr(filters ...Filter) Filter {
	return itypes.FilterOr(filters...)
}

// FilterAnd groups filters that are AND-combined with each other. Filters are
// already AND-combined at the top level, so the group exists to nest an AND
// inside an OR group.
func FilterAnd(filters ...Filter) Filter {
	return itypes.FilterAnd(filters...)
}

// FilterExists matches rows of the queried model that have at least one
// related row in C satisfying filters. EqCol predicates tie the related
// rows to the queried row, one per column pair, next to the ordinary
// conditions narrowing them.
func FilterExists[C Model](filters ...Filter) Filter {
	return itypes.FilterExists[C](filters...)
}

// FilterNotExists matches rows that have no related row in C satisfying
// filters. Note that it is not the negation of a filtered FilterExists over the
// same rows: a row whose related rows all fail filters matches, and so does a
// row with no related rows at all. A subquery without any EqCol predicate
// fails closed here as well: negating "match nothing" would otherwise widen
// into "match everything".
func FilterNotExists[C Model](filters ...Filter) Filter {
	return itypes.FilterNotExists[C](filters...)
}
