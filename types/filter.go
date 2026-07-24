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

// Service-only operators: for service code building Filters
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

// Filter is one field-level filter to apply as an AND condition.
// Column must already be validated against the model's queryable columns by
// the producer (the List controller validates URL input; service code passing
// filters directly carries the same responsibility). Value holds a normalized
// typed value and is always bound as a statement parameter:
//
//   - FilterOpIn and FilterOpNotIn require a slice or array value.
//   - FilterOpIsNull requires a bool value.
//   - FilterOpLike, FilterOpNotLike, FilterOpStartsWith, FilterOpEndsWith,
//     FilterOpRegex, FilterOpNotRegex, and FilterOpJSONContains require a
//     string value.
//   - The comparison operators take a scalar value (string, numeric,
//     time.Time); slices, arrays, and nil are rejected.
//
// A value that violates these rules fails closed in the database layer.
// Service code should build filters with the FilterEq/FilterIn/... helper
// constructors: their signatures enforce the value shape at compile time.
type Filter struct {
	Column string
	Op     FilterOp
	Value  any
}

// The Filter constructors below build one Filter per operator and are the
// intended way for service code to produce filters: each signature locks the
// value shape its operator expects, so a malformed filter cannot be expressed
// without bypassing the constructors. Column is a snake case column name;
// validating it against the model's queryable columns remains the caller's
// responsibility.

// FilterEq matches rows where column equals value.
func FilterEq(column string, value any) Filter {
	return Filter{Column: column, Op: FilterOpEq, Value: value}
}

// FilterNe matches rows where column does not equal value.
func FilterNe(column string, value any) Filter {
	return Filter{Column: column, Op: FilterOpNe, Value: value}
}

// FilterGt matches rows where column is greater than value.
func FilterGt(column string, value any) Filter {
	return Filter{Column: column, Op: FilterOpGt, Value: value}
}

// FilterGte matches rows where column is greater than or equal to value.
func FilterGte(column string, value any) Filter {
	return Filter{Column: column, Op: FilterOpGte, Value: value}
}

// FilterLt matches rows where column is less than value.
func FilterLt(column string, value any) Filter {
	return Filter{Column: column, Op: FilterOpLt, Value: value}
}

// FilterLte matches rows where column is less than or equal to value.
func FilterLte(column string, value any) Filter {
	return Filter{Column: column, Op: FilterOpLte, Value: value}
}

// FilterIn matches rows where column is one of values. The slice is bound as
// a whole; an empty slice matches nothing.
func FilterIn[T any](column string, values []T) Filter {
	return Filter{Column: column, Op: FilterOpIn, Value: values}
}

// FilterNotIn matches rows where column is none of values. The slice is
// bound as a whole; an empty slice matches nothing (SQL NOT IN over an empty
// list never holds), it does not mean "exclude nothing".
func FilterNotIn[T any](column string, values []T) Filter {
	return Filter{Column: column, Op: FilterOpNotIn, Value: values}
}

// FilterLike matches rows where column contains value as a substring; value
// is escaped and matches literally.
func FilterLike(column, value string) Filter {
	return Filter{Column: column, Op: FilterOpLike, Value: value}
}

// FilterNotLike matches rows where column does not contain value as a
// substring; value is escaped and matches literally.
func FilterNotLike(column, value string) Filter {
	return Filter{Column: column, Op: FilterOpNotLike, Value: value}
}

// FilterStartsWith matches rows where column starts with value; value is
// escaped and matches literally, and the prefix form can use an index.
func FilterStartsWith(column, value string) Filter {
	return Filter{Column: column, Op: FilterOpStartsWith, Value: value}
}

// FilterEndsWith matches rows where column ends with value; value is escaped
// and matches literally.
func FilterEndsWith(column, value string) Filter {
	return Filter{Column: column, Op: FilterOpEndsWith, Value: value}
}

// FilterIsNull matches rows whose column is NULL.
func FilterIsNull(column string) Filter {
	return Filter{Column: column, Op: FilterOpIsNull, Value: true}
}

// FilterNotNull matches rows whose column is not NULL.
func FilterNotNull(column string) Filter {
	return Filter{Column: column, Op: FilterOpIsNull, Value: false}
}

// FilterRegex matches rows where column matches the regular expression expr
// (dialect-aware REGEXP).
func FilterRegex(column, expr string) Filter {
	return Filter{Column: column, Op: FilterOpRegex, Value: expr}
}

// FilterNotRegex matches rows where column does not match the regular
// expression expr.
func FilterNotRegex(column, expr string) Filter {
	return Filter{Column: column, Op: FilterOpNotRegex, Value: expr}
}

// FilterJSONContains matches rows whose JSON array column contains value as
// a member.
func FilterJSONContains(column, value string) Filter {
	return Filter{Column: column, Op: FilterOpJSONContains, Value: value}
}
