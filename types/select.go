package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// Selector runs an analytical read over the table of M and scans the result
// rows into R. It is deliberately separate from Database[M]: a projected row
// is not a model row, so model hooks, association preloading and cursor
// pagination have nothing to act on and are absent here rather than present
// and inert.
type Selector[M Model, R any] = itypes.Selector[M, R]
