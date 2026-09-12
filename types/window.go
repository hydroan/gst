package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// Window names the rows a window function reads for each row: PartitionBy
// splits the rows into partitions, and OrderBy orders each partition, which is
// what gives a running total its direction and a row number its sequence.
type Window = itypes.Window

// PartitionBy opens a window partitioned by keys. Without keys the whole
// result is one partition, which is what a ranking over every row wants, and
// the window is the one OrderBy opens: the two spellings build the same
// value, so a term declared with one is found by the other.
func PartitionBy(keys ...Expr) Window {
	return itypes.PartitionBy(keys...)
}

// OrderBy returns a Window with no partition and the given orders: the
// window a ranking across every row reads. It is the short spelling of
// PartitionBy().OrderBy(orders...), the two building the same window; a
// window with keys starts from PartitionBy. It orders the window, not the
// result: the result is ordered by the Selector's OrderBy.
func OrderBy(orders ...Ordering) Window {
	return itypes.OrderBy(orders...)
}
