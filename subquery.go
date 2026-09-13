package gst

import (
	"github.com/hydroan/gst/internal/types"
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
