package gst

import (
	"github.com/hydroan/gst/internal/types"
)

// FilterOp is a field-level filter operator: the comparison a Filter applies,
// which Filter.Op reads back. Operators never widen a query: unknown values
// are rejected during parsing, and the database layer fails closed on
// conditions it does not recognize.
type FilterOp = types.FilterOp

// URL-exposed operators, which a request spells as "field[op]=value".
const (
	FilterOpEq         = types.FilterOpEq         // equal: column = value
	FilterOpNe         = types.FilterOpNe         // not equal: column <> value
	FilterOpGt         = types.FilterOpGt         // greater than: column > value
	FilterOpGte        = types.FilterOpGte        // greater than or equal: column >= value
	FilterOpLt         = types.FilterOpLt         // less than: column < value
	FilterOpLte        = types.FilterOpLte        // less than or equal: column <= value
	FilterOpIn         = types.FilterOpIn         // set membership: column IN (comma-separated values)
	FilterOpNotIn      = types.FilterOpNotIn      // set exclusion: column NOT IN (comma-separated values)
	FilterOpLike       = types.FilterOpLike       // substring match: column LIKE %value%
	FilterOpNotLike    = types.FilterOpNotLike    // substring exclusion: column NOT LIKE %value%
	FilterOpStartsWith = types.FilterOpStartsWith // prefix match: column LIKE value% (can use an index)
	FilterOpEndsWith   = types.FilterOpEndsWith   // suffix match: column LIKE %value
	FilterOpIsNull     = types.FilterOpIsNull     // null check: value true means IS NULL, false means IS NOT NULL
)

// Service-only operators, which service code builds and no request can spell.
const (
	FilterOpRegex        = types.FilterOpRegex        // regular expression match: column REGEXP value (dialect-aware)
	FilterOpNotRegex     = types.FilterOpNotRegex     // regular expression exclusion: NOT (column REGEXP value)
	FilterOpJSONContains = types.FilterOpJSONContains // JSON array membership: value is a member of the JSON array column
	FilterOpOr           = types.FilterOpOr           // group: the []Filter value is OR-combined, the group itself AND-combined
	FilterOpAnd          = types.FilterOpAnd          // group: the []Filter value is AND-combined, for nesting inside an OR group
	FilterOpExists       = types.FilterOpExists       // correlated subquery: EXISTS or NOT EXISTS over a related model
	FilterOpEqCol        = types.FilterOpEqCol        // column equals another column: the enclosing query's inside a subquery, a table read beside it inside a join
	FilterOpFalse        = types.FilterOpFalse        // constant predicate: matches nothing, see FilterFalse
)
