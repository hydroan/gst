package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// Assignment is one column-value write, the unit UpdateByID accepts. Service
// code builds assignments through the generated column references
// (SampleCols.Status.Set(v)), whose typed front end stops a wrong-typed value
// or a misspelled column at compile time; generic code assigns through a
// reference minted for its type parameter.
type Assignment = itypes.Assignment
