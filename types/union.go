package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// SelectBranch is a select in the role of a branch of a union: every Selector
// is one, with its model type erased, so that selects over different models
// stack into one result as long as they scan into the same row type. The
// role is what UnionAll takes. Only the selects the database layer builds can
// fill it; UnionAll fails when handed anything else, a union among them.
type SelectBranch[R any] = itypes.SelectBranch[R]

// Union stacks the rows of several selects into one result: UNION ALL, the
// one set operation the framework offers. UNION proper would fold two rows
// that happen to be equal — two payments of the same amount on the same day
// — into one, which no report wants; INTERSECT and EXCEPT are the semi joins
// FilterExists and FilterNotExists already express.
type Union[R any] = itypes.Union[R]
