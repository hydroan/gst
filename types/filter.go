package types

// FilterOp is a field-level filter operator applied by WithQuery as an
// additional AND condition. Operators never widen a query: unknown values
// are rejected during parsing, and the database layer fails closed on
// conditions it does not recognize.
//
// Operators come in two tiers, and the split is load-bearing:
//
//   - URL-exposed operators are registered in the filterOps parse map and
//     carried by the List/Export query parameter syntax "field[op]=value";
//     FilterOps returns them for API documentation.
//   - Service-only operators exist as constants and execute in the database
//     layer, but are intentionally absent from the parse map: they never
//     validate column types or values at the URL boundary, so exposing one
//     requires adding that validation first, not just registering the name.
type FilterOp string

// URL-exposed operators.
const (
	FilterOpEq         FilterOp = "eq"         // equal: column = value
	FilterOpNe         FilterOp = "ne"         // not equal: column <> value
	FilterOpGt         FilterOp = "gt"         // greater than: column > value
	FilterOpGte        FilterOp = "gte"        // greater than or equal: column >= value
	FilterOpLt         FilterOp = "lt"         // less than: column < value
	FilterOpLte        FilterOp = "lte"        // less than or equal: column <= value
	FilterOpIn         FilterOp = "in"         // set membership: column IN (comma-separated values)
	FilterOpNotIn      FilterOp = "notin"      // set exclusion: column NOT IN (comma-separated values)
	FilterOpLike       FilterOp = "like"       // substring match: column LIKE %value%
	FilterOpNotLike    FilterOp = "notlike"    // substring exclusion: column NOT LIKE %value%
	FilterOpStartsWith FilterOp = "startswith" // prefix match: column LIKE value% (can use an index)
	FilterOpEndsWith   FilterOp = "endswith"   // suffix match: column LIKE %value
	FilterOpIsNull     FilterOp = "isnull"     // null check: value true means IS NULL, false means IS NOT NULL
)

// Service-only operators: for service code building FieldConditions
// directly, reusable and injection-safe alternatives to raw SQL fragments.
const (
	FilterOpRegex        FilterOp = "regex"        // regular expression match: column REGEXP value (dialect-aware)
	FilterOpNotRegex     FilterOp = "notregex"     // regular expression exclusion: NOT (column REGEXP value)
	FilterOpJSONContains FilterOp = "jsoncontains" // JSON array membership: value is a member of the JSON array column
)

// filterOps indexes the URL-exposed operators for parsing; service-only
// operators are deliberately absent (see the FilterOp tier note). Matching is
// exact and case-sensitive: URL query keys are contract surface, not
// free-form input.
var filterOps = map[string]FilterOp{
	string(FilterOpEq):         FilterOpEq,
	string(FilterOpNe):         FilterOpNe,
	string(FilterOpGt):         FilterOpGt,
	string(FilterOpGte):        FilterOpGte,
	string(FilterOpLt):         FilterOpLt,
	string(FilterOpLte):        FilterOpLte,
	string(FilterOpIn):         FilterOpIn,
	string(FilterOpNotIn):      FilterOpNotIn,
	string(FilterOpLike):       FilterOpLike,
	string(FilterOpNotLike):    FilterOpNotLike,
	string(FilterOpStartsWith): FilterOpStartsWith,
	string(FilterOpEndsWith):   FilterOpEndsWith,
	string(FilterOpIsNull):     FilterOpIsNull,
}

// ParseFilterOp converts an operator token from a "field[op]" query key into
// a FilterOp, reporting whether the token is a known operator.
func ParseFilterOp(s string) (FilterOp, bool) {
	op, ok := filterOps[s]
	return op, ok
}

// FilterOps returns every URL-exposed operator in a stable order, for API
// documentation surfaces such as the generated OpenAPI parameter notes.
// Service-only operators are excluded on purpose: they are not part of the
// URL contract.
func FilterOps() []FilterOp {
	return []FilterOp{
		FilterOpEq, FilterOpNe,
		FilterOpGt, FilterOpGte, FilterOpLt, FilterOpLte,
		FilterOpIn, FilterOpNotIn,
		FilterOpLike, FilterOpNotLike, FilterOpStartsWith, FilterOpEndsWith,
		FilterOpIsNull,
	}
}

// FieldCondition is one field-level filter to apply as an AND condition.
// Column must already be validated against the model's queryable columns by
// the producer (the List controller validates URL input; service code passing
// conditions directly carries the same responsibility). Value is always bound
// as a statement parameter; FilterOpIn and FilterOpNotIn split it on commas.
type FieldCondition struct {
	Column string
	Op     FilterOp
	Value  string
}
