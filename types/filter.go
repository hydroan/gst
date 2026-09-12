package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// FilterOp is a field-level filter operator applied by WithQuery as an
// additional AND condition. Operators never widen a query: unknown values
// are rejected during parsing, and the database layer fails closed on
// conditions it does not recognize.
type FilterOp = itypes.FilterOp

// URL-exposed operators.
const (
	FilterOpEq         = itypes.FilterOpEq         // equal: column = value
	FilterOpNe         = itypes.FilterOpNe         // not equal: column <> value
	FilterOpGt         = itypes.FilterOpGt         // greater than: column > value
	FilterOpGte        = itypes.FilterOpGte        // greater than or equal: column >= value
	FilterOpLt         = itypes.FilterOpLt         // less than: column < value
	FilterOpLte        = itypes.FilterOpLte        // less than or equal: column <= value
	FilterOpIn         = itypes.FilterOpIn         // set membership: column IN (comma-separated values)
	FilterOpNotIn      = itypes.FilterOpNotIn      // set exclusion: column NOT IN (comma-separated values)
	FilterOpLike       = itypes.FilterOpLike       // substring match: column LIKE %value%
	FilterOpNotLike    = itypes.FilterOpNotLike    // substring exclusion: column NOT LIKE %value%
	FilterOpStartsWith = itypes.FilterOpStartsWith // prefix match: column LIKE value% (can use an index)
	FilterOpEndsWith   = itypes.FilterOpEndsWith   // suffix match: column LIKE %value
	FilterOpIsNull     = itypes.FilterOpIsNull     // null check: value true means IS NULL, false means IS NOT NULL
)

// Service-only operators: for service code building Filters
// directly, reusable and injection-safe alternatives to raw SQL fragments.
const (
	FilterOpRegex        = itypes.FilterOpRegex        // regular expression match: column REGEXP value (dialect-aware)
	FilterOpNotRegex     = itypes.FilterOpNotRegex     // regular expression exclusion: NOT (column REGEXP value)
	FilterOpJSONContains = itypes.FilterOpJSONContains // JSON array membership: value is a member of the JSON array column
	FilterOpOr           = itypes.FilterOpOr           // group: the []Filter value is OR-combined, the group itself AND-combined
	FilterOpAnd          = itypes.FilterOpAnd          // group: the []Filter value is AND-combined, for nesting inside an OR group
	FilterOpExists       = itypes.FilterOpExists       // correlated subquery: the Subquery value becomes EXISTS or NOT EXISTS
	FilterOpEqCol        = itypes.FilterOpEqCol        // column equals another column, named by the value as a plain name or a column reference: the enclosing query's inside a subquery, a table read beside it inside a join
	FilterOpFalse        = itypes.FilterOpFalse        // constant predicate: matches nothing, see FilterFalse
)

// ParseFilterOp converts an operator token from a "field[op]" query key into
// a FilterOp, reporting whether the token is a known operator.
func ParseFilterOp(s string) (FilterOp, bool) {
	return itypes.ParseFilterOp(s)
}

// FilterOps returns every URL-exposed operator in a stable order, for API
// documentation surfaces such as the generated OpenAPI parameter notes.
// Service-only operators are excluded on purpose: they are not part of the
// URL contract.
func FilterOps() []FilterOp {
	return itypes.FilterOps()
}

// Filter is one field-level filter to apply as an AND condition.
// Column must already be validated against the model's queryable columns by
// the producer (the List controller validates URL input; service code passing
// filters directly carries the same responsibility). Table is the table the
// column belongs to: a column reference fills it in, the string constructors
// and URL parsing leave it empty, which names the queried model's own table.
// A filter carrying another table is applied to that table when the query
// joins it and fails closed otherwise. Value holds a normalized typed value
// and is always bound as a statement parameter.
type Filter = itypes.Filter

// FilterTimeLayout is the canonical layout a time-typed filter value parsed
// from a URL is normalized to. The value travels as a string rather than a
// time.Time on purpose: binding a time.Time would let the driver re-render it
// in its own location, while the string pins the wall-clock time the parser
// resolved. The pinned wall clock is UTC, the one wall clock the framework
// stores on every dialect.
const FilterTimeLayout = itypes.FilterTimeLayout

// FilterEq matches rows where column equals value.
func FilterEq(column string, value any) Filter {
	return itypes.FilterEq(column, value)
}

// FilterNe matches rows where column does not equal value.
func FilterNe(column string, value any) Filter {
	return itypes.FilterNe(column, value)
}

// FilterGt matches rows where column is greater than value.
func FilterGt(column string, value any) Filter {
	return itypes.FilterGt(column, value)
}

// FilterGte matches rows where column is greater than or equal to value.
func FilterGte(column string, value any) Filter {
	return itypes.FilterGte(column, value)
}

// FilterLt matches rows where column is less than value.
func FilterLt(column string, value any) Filter {
	return itypes.FilterLt(column, value)
}

// FilterLte matches rows where column is less than or equal to value.
func FilterLte(column string, value any) Filter {
	return itypes.FilterLte(column, value)
}

// FilterIn matches rows where column is one of values. The slice is bound as
// a whole; an empty slice matches nothing.
func FilterIn[T any](column string, values []T) Filter {
	return itypes.FilterIn[T](column, values)
}

// FilterNotIn matches rows where column is none of values. The slice is
// bound as a whole; an empty slice matches nothing (SQL NOT IN over an empty
// list never holds), it does not mean "exclude nothing".
func FilterNotIn[T any](column string, values []T) Filter {
	return itypes.FilterNotIn[T](column, values)
}

// FilterLike matches rows where column contains value as a substring; value
// is escaped and matches literally.
func FilterLike(column string, value string) Filter {
	return itypes.FilterLike(column, value)
}

// FilterNotLike matches rows where column does not contain value as a
// substring; value is escaped and matches literally.
func FilterNotLike(column string, value string) Filter {
	return itypes.FilterNotLike(column, value)
}

// FilterStartsWith matches rows where column starts with value; value is
// escaped and matches literally, and the prefix form can use an index.
func FilterStartsWith(column string, value string) Filter {
	return itypes.FilterStartsWith(column, value)
}

// FilterEndsWith matches rows where column ends with value; value is escaped
// and matches literally.
func FilterEndsWith(column string, value string) Filter {
	return itypes.FilterEndsWith(column, value)
}

// FilterIsNull matches rows whose column is NULL.
func FilterIsNull(column string) Filter {
	return itypes.FilterIsNull(column)
}

// FilterIsNotNull matches rows whose column is not NULL.
func FilterIsNotNull(column string) Filter {
	return itypes.FilterIsNotNull(column)
}

// FilterRegex matches rows where column matches the regular expression expr
// (dialect-aware REGEXP).
func FilterRegex(column string, expr string) Filter {
	return itypes.FilterRegex(column, expr)
}

// FilterNotRegex matches rows where column does not match the regular
// expression expr.
func FilterNotRegex(column string, expr string) Filter {
	return itypes.FilterNotRegex(column, expr)
}

// FilterJSONContains matches rows whose JSON array column contains value as
// a member.
func FilterJSONContains(column string, value string) Filter {
	return itypes.FilterJSONContains(column, value)
}

// FilterFalse matches nothing. It is the condition a permission hook returns
// when the caller may see no row at all. Unlike an empty filter list it is a
// real condition, so it disables the empty-query safety check; unlike a
// filter the renderer cannot apply it is deliberate, so nothing is logged. It
// renders as 1 = 0 on every dialect and composes like any other filter,
// inside groups, subqueries and conditional measures included.
func FilterFalse() Filter {
	return itypes.FilterFalse()
}

// FilterOr groups filters that are OR-combined with each other. The group as a
// whole stays AND-combined with every other condition of the query, so a
// mandatory condition such as tenant scoping can never be absorbed into the
// alternatives.
func FilterOr(filters ...Filter) Filter {
	return itypes.FilterOr(filters...)
}

// FilterAnd groups filters that are AND-combined with each other. Filters are
// already AND-combined at the top level, so the group exists to nest an AND
// inside an OR group.
func FilterAnd(filters ...Filter) Filter {
	return itypes.FilterAnd(filters...)
}

// FilterEqCol is the predicate that ties two tables together by a column
// each: inside FilterExists or FilterNotExists, column on the related model
// equals parent on the enclosing query's model, rendered as
// `child_table.column = outer_table.parent`; inside a Join, it is the ON
// condition. At the top level of a query without a join there is nothing to
// tie to and it fails closed, as does an empty name on either side or a name
// the related or the enclosing model does not have. Several of them express a
// composite key, and one inside a FilterOr group matches on any of its pairs.
// Inside a join only the pairs at the top level of the ON count toward the
// key the join is proved unique on: one inside a FilterOr group narrows the
// match but proves nothing.
func FilterEqCol(column string, parent string) Filter {
	return itypes.FilterEqCol(column, parent)
}

// Subquery is the correlated EXISTS subquery carried by FilterOpExists. It
// names the related model and the predicates narrowing its rows, at least one
// of which must be a FilterEqCol tying them to the enclosing query.
type Subquery = itypes.Subquery

// FilterExists matches rows of the queried model that have at least one
// related row in C satisfying filters. EqCol predicates tie the related
// rows to the queried row, one per column pair, next to the ordinary
// conditions narrowing them.
func FilterExists[C Model](filters ...Filter) Filter {
	return itypes.FilterExists[C](filters...)
}

// FilterNotExists matches rows that have no related row in C satisfying
// filters. Note that it is not the negation of a filtered FilterExists over the
// same rows: a row whose related rows all fail filters matches, and so does a
// row with no related rows at all. A subquery without any FilterEqCol
// fails closed here as well: negating "match nothing" would otherwise widen
// into "match everything".
func FilterNotExists[C Model](filters ...Filter) Filter {
	return itypes.FilterNotExists[C](filters...)
}
