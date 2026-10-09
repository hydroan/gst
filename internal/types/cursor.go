package types

// Cursor is where a cursor-paginated read starts and which way it goes: the
// feed's stable ordering, the boundary row, and whether the read travels along
// that ordering or back down it.
//
// This is the database argument. model.Cursor is the separate struct a model
// embeds to opt in to the cursor URL parameters that produce it. The fields
// are unexported so a cursor only comes from CursorForward, CursorBackward and
// the framework's URL parsing; Order, Value and Backward read it back.
//
// The cursor orders the feed on every page, the first one included: a
// cursor without a boundary value reads the feed from its start, in its
// order, so the row a client pages on from is the row the order puts last.
// Traveling backward reverses both the boundary comparison and the ORDER BY,
// and List reverses the returned rows afterwards, so a backward page comes
// back in the feed's own order rather than upside down:
//
//	feed ASC,  forward   -> column > value, ORDER BY column ASC
//	feed ASC,  backward  -> column < value, ORDER BY column DESC, rows reversed
//	feed DESC, forward   -> column < value, ORDER BY column DESC
//	feed DESC, backward  -> column > value, ORDER BY column ASC,  rows reversed
//
// URL-driven cursors always page an ascending feed; a descending feed is a
// service-side cursor, built with CursorForward on a Desc order.
type Cursor struct {
	// order is the feed's stable ordering. An empty column falls back to the
	// primary key in the database layer.
	order Order
	// value is the boundary row's column value. An empty value reads the
	// feed from its start, or from its end when traveling backward.
	value string
	// backward travels against order instead of along it, which is what a
	// request for the previous page means.
	backward bool
	// paging is set by the constructors; the zero Cursor pages nothing and
	// leaves a query as it is.
	paging bool
}

// CursorForward pages along order, starting just past value, or at the
// feed's first row when value is empty.
func CursorForward(order Order, value string) Cursor {
	return Cursor{order: order, value: value, paging: true}
}

// CursorBackward pages against order, starting just before value, or at the
// feed's last row when value is empty.
func CursorBackward(order Order, value string) Cursor {
	return Cursor{order: order, value: value, backward: true, paging: true}
}

// Order returns the feed's stable ordering. An empty column in it falls back
// to the primary key in the database layer.
func (c Cursor) Order() Order { return c.order }

// Value returns the boundary row's column value; an empty value starts the
// read at the feed's own start, or at its end when traveling backward (see
// Bounded).
func (c Cursor) Value() string { return c.value }

// Backward reports whether the read travels against the ordering, which is
// what a request for the previous page means.
func (c Cursor) Backward() bool { return c.backward }

// Enabled reports whether the cursor pages the read: it orders the feed by
// its column and, when Bounded, starts past the boundary. The zero Cursor
// reports false and leaves the query as it is.
func (c Cursor) Enabled() bool { return c.paging }

// Bounded reports whether the cursor carries a boundary value to start past;
// the first page of a feed has none.
func (c Cursor) Bounded() bool { return len(c.value) > 0 }
