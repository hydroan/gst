package types

// Window names the rows a window function reads for each row: PartitionBy
// splits the rows into partitions, and OrderBy orders each partition, which is
// what gives a running total its direction and a row number its sequence.
//
// The window is rendered by the database layer, which completes an ordered
// window with a tie breaker — the primary key of a row-level projection, the
// group keys of a grouped one — so two rows sorting equal still take a stable
// order, and fixes the frame of an ordered aggregate to the rows from the
// partition's first up to the current one. Neither is a knob: the SQL
// defaults they replace are the two classic ways a running total goes wrong.
// Rank and DenseRank take no tie breaker: ranking peers equally is what they
// are for.
//
//	RowNumber().Over(PartitionBy(SampleCols.TenantID).OrderBy(SampleCols.CreatedAt.Desc()))
//	// ROW_NUMBER() OVER (PARTITION BY `tenant_id` ORDER BY `created_at` DESC, `id` ASC)
//	Rank().Over(OrderBy(total.Desc()))
//	// RANK() OVER (ORDER BY COALESCE(SUM(`amount`), 0) DESC)
type Window struct {
	// Partition holds the partition keys. Each is a column of a table the
	// select reads, the queried model's or a joined one's, in a row-level
	// projection, or one of the projection's group keys in a grouped one; a
	// joined select's term the projection reads, and the time buckets, work
	// in both.
	Partition []Term
	// Orders sorts each partition. A column order names a column of a table
	// the select reads in a row-level projection, or a group key in a grouped
	// one; a term order names a projected term, which is how a grouped
	// projection ranks by a measure.
	Orders []Ordering
}

// PartitionBy opens a window partitioned by keys. Without keys the whole
// result is one partition, which is what a ranking over every row wants, and
// the window is the one OrderBy opens: the two spellings build the same
// value, so a term declared with one is found by the other.
func PartitionBy(keys ...Expr) Window {
	if len(keys) == 0 {
		return Window{}
	}
	terms := make([]Term, 0, len(keys))
	for _, key := range keys {
		terms = append(terms, key.exprTerm())
	}
	return Window{Partition: terms}
}

// OrderBy returns a Window with no partition and the given orders: the
// window a ranking across every row reads. It is the short spelling of
// PartitionBy().OrderBy(orders...), the two building the same window; a
// window with keys starts from PartitionBy. It orders the window, not the
// result: the result is ordered by the Selector's OrderBy.
func OrderBy(orders ...Ordering) Window {
	return Window{}.OrderBy(orders...)
}

// OrderBy orders each partition of the window.
func (w Window) OrderBy(orders ...Ordering) Window {
	w.Orders = append(append([]Ordering(nil), w.Orders...), orders...)
	return w
}
