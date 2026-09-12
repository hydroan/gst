package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// Database defines the model-scoped database operation contract.
// It provides CRUD operations, query builders, and optional dry-run behavior
// for a single Model type.
type Database[M Model] = itypes.Database[M]

// DatabaseOption provides chainable options for a single Database operation chain.
// Options apply to the next terminal operation and are reset afterward. Start a
// new chain with database.Database[M](ctx) for each independent operation.
type DatabaseOption[M Model] = itypes.DatabaseOption[M]

// QueryOptions tunes how WithQuery turns a model value into WHERE conditions.
// Every condition it produces is AND-combined; the zero value means exact
// matching with the empty-query safety check enabled. See the WithQuery method
// for usage examples.
type QueryOptions = itypes.QueryOptions

// SQLStatement contains a generated SQL statement in executable and rendered forms.
type SQLStatement = itypes.SQLStatement
