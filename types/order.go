package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// Order is one ORDER BY term: a column and the direction to sort it by.
// Column must already be validated against the model's queryable columns by
// the producer (the List controller validates URL input; service code passing
// orders directly carries the same responsibility). Table is the table the
// column belongs to: a column reference fills it in, and URL parsing leaves it
// empty. The chain's reads and a select check it: a select that joins tells
// two tables' columns of one name apart by it, and a chain refuses an order of
// another model. A union orders its result columns by name and reads no table,
// and WithExpand orders the associated table, so neither checks it. An Order
// with an empty column is skipped rather than rendered.
type Order = itypes.Order

// Ordering is what the OrderBy methods of a select, a window and a union
// accept: an Order sorting by a column reference, or a TermOrder sorting by a
// projection term. The set is closed, so an ordering can never carry SQL the
// way a free-form string could.
type Ordering = itypes.Ordering
