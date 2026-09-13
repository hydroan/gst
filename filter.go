package gst

import (
	"github.com/hydroan/gst/internal/types"
)

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
