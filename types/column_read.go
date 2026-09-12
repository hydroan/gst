package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// Bound is one end of the range a column's comparison filters confine it to:
// the value at that end, whether the range includes it, and whether any filter
// gave that end at all. An absent end leaves the range open on that side.
type Bound[T any] = itypes.Bound[T]
