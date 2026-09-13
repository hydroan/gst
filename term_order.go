package gst

import (
	"github.com/hydroan/gst/internal/types"
)

// TermOrder is one ORDER BY term of a select or of a window. Unlike Order it
// sorts by a projection term, which is what a TopN report ranks by.
type TermOrder = types.TermOrder
