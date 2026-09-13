package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// Filter is one field-level filter to apply as an AND condition.
// Column must already be validated against the model's queryable columns by
// the producer (the List controller validates URL input; service code passing
// filters directly carries the same responsibility). Table is the table the
// column belongs to: a column reference fills it in, and URL parsing leaves it
// empty, which names the queried model's own table. A filter carrying another
// table is applied to that table when the query joins it and fails closed
// otherwise. Value holds a normalized typed value and is always bound as a
// statement parameter.
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
