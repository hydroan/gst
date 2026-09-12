package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// Cursor is where a cursor-paginated read starts and which way it goes: the
// feed's stable ordering, the boundary row, and whether the read travels along
// that ordering or back down it.
type Cursor = itypes.Cursor

// CursorForward pages along order, starting just past value.
func CursorForward(order Order, value string) Cursor {
	return itypes.CursorForward(order, value)
}

// CursorBackward pages against order, starting just before value.
func CursorBackward(order Order, value string) Cursor {
	return itypes.CursorBackward(order, value)
}
