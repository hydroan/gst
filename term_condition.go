package gst

import (
	"github.com/hydroan/gst/internal/types"
)

// TermCondition is one condition on a projected term: a Having condition on a
// measure, or a Qualify condition on a window function. It carries the term
// itself rather than an alias string, which has two consequences: a condition
// can never name a term the projection did not declare, and the renderer can
// emit the full expression instead of the alias, which HAVING requires because
// PostgreSQL does not accept an output alias there.
type TermCondition = types.TermCondition
