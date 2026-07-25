package types

// CursorPosition is where a cursor-paginated read starts and which way it
// goes: the feed's stable ordering, the boundary row, and whether the read
// travels along that ordering or back down it.
//
// The type is named CursorPosition rather than Cursor because model.Cursor
// already names the struct a model embeds to opt in to the cursor URL
// parameters; this one is the resulting database argument.
//
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
type CursorPosition struct {
	// Order is the feed's stable ordering. An empty column falls back to the
	// primary key in the database layer.
	Order Order
	// Value is the boundary row's column value. An empty value disables
	// cursor pagination, which makes a zero CursorPosition a no-op.
	Value string
	// Backward travels against Order instead of along it, which is what a
	// request for the previous page means.
	Backward bool
}

// CursorForward pages along order, starting just past value.
func CursorForward(order Order, value string) CursorPosition {
	return CursorPosition{Order: order, Value: value}
}

// CursorBackward pages against order, starting just before value.
func CursorBackward(order Order, value string) CursorPosition {
	return CursorPosition{Order: order, Value: value, Backward: true}
}

// Enabled reports whether the cursor carries a boundary and should be applied.
func (c CursorPosition) Enabled() bool { return len(c.Value) > 0 }
