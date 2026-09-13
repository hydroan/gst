package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

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
type Order = itypes.Order

// Ordering is what the OrderBy methods of a select, a window and a union
// accept: an Order sorting by a column reference, or a TermOrder sorting by a
// projection term. The set is closed, so an ordering can never carry SQL the
// way a free-form string could.
type Ordering = itypes.Ordering
