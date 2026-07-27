package types

import (
	"reflect"
	"time"
)

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
	FilterOpOr           FilterOp = "or"           // group: the []Filter value is OR-combined, the group itself AND-combined
	FilterOpAnd          FilterOp = "and"          // group: the []Filter value is AND-combined, for nesting inside an OR group
	FilterOpExists       FilterOp = "exists"       // correlated subquery: the Subquery value becomes EXISTS or NOT EXISTS
)

// Subquery is the correlated EXISTS subquery carried by FilterOpExists. It
// names the related model, the column pair that correlates it with the outer
// query, and the conditions narrowing it.
//
// A semi join is used rather than a real join on purpose: EXISTS matches a row
// at most once, so an aggregate over the outer table keeps counting each row
// once. A join to a one-to-many child multiplies the outer rows instead, and a
// SUM over that silently doubles.
type Subquery struct {
	// Model is an allocated instance of the related model. It carries the
	// child table name and its soft-delete scope, so a subquery hides the same
	// rows a List on that model hides.
	Model Model
	// ChildColumn is the correlated column on the related model.
	ChildColumn string
	// ParentColumn is the correlated column on the model being queried.
	ParentColumn string
	// Filters narrow the related rows.
	Filters []Filter
	// Negate turns the condition into NOT EXISTS.
	Negate bool
}

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
//   - FilterOpOr and FilterOpAnd require a non-empty []Filter value and carry
//     no column: they group their children instead of naming one themselves.
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

// FilterTimeLayout is the canonical layout a time-typed filter value parsed
// from a URL is normalized to. The value travels as a string rather than a
// time.Time on purpose: binding a time.Time would let the driver re-render it
// in its own location, while the string pins the wall-clock time the parser
// resolved.
//
// It is exported because the normalization is a contract of Filter, not a
// detail of the parser: a service reading a bound back would otherwise have to
// restate the layout, which no compiler could keep in sync with the parser.
// Read a bound with TimeValue rather than parsing with this layout directly.
const FilterTimeLayout = "2006-01-02 15:04:05.999999999"

// TimeValue returns the filter's value as a time, which is how a service reads
// back a range it did not build itself, such as one parsed from a request.
//
// Both value shapes a time bound can have are accepted: the canonical string a
// URL-parsed filter carries, and the time.Time a caller passes to the
// comparison constructors directly. It reports false for a value that is
// neither, including a malformed string, so a caller that must distinguish
// "no bound" from "some other value" can.
func (f Filter) TimeValue() (time.Time, bool) {
	switch value := f.Value.(type) {
	case time.Time:
		return value, true
	case string:
		parsed, err := time.ParseInLocation(FilterTimeLayout, value, time.Local)
		if err != nil {
			return time.Time{}, false
		}
		return parsed, true
	default:
		return time.Time{}, false
	}
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

// FilterOr groups filters that are OR-combined with each other. The group as a
// whole stays AND-combined with every other condition of the query, so a
// mandatory condition such as tenant scoping can never be absorbed into the
// alternatives:
//
//	Filters: []types.Filter{
//	    types.FilterEq("tenant_id", tenant),
//	    types.FilterOr(
//	        types.FilterLike("name", keyword),
//	        types.FilterLike("code", keyword),
//	    ),
//	}
//	// WHERE tenant_id = ? AND (name LIKE ? OR code LIKE ?)
//
// Children may themselves be groups, which is how nesting is expressed; see
// FilterAnd for the "(a AND b) OR (c AND d)" shape. A group with no children
// fails closed.
func FilterOr(filters ...Filter) Filter {
	return Filter{Op: FilterOpOr, Value: filters}
}

// FilterAnd groups filters that are AND-combined with each other. Filters are
// already AND-combined at the top level, so the group exists to nest an AND
// inside an OR group:
//
//	Filters: []types.Filter{
//	    types.FilterEq("tenant_id", tenant),
//	    types.FilterOr(
//	        types.FilterAnd(
//	            types.FilterEq("kind", KindPrimary),
//	            types.FilterEq("status", StatusDone),
//	        ),
//	        types.FilterAnd(
//	            types.FilterEq("kind", KindSecondary),
//	            types.FilterEq("status", StatusPending),
//	        ),
//	    ),
//	}
//	// WHERE tenant_id = ?
//	//   AND ((kind = ? AND status = ?) OR (kind = ? AND status = ?))
//
// A group with no children fails closed.
func FilterAnd(filters ...Filter) Filter {
	return Filter{Op: FilterOpAnd, Value: filters}
}

// FilterExists matches rows of the queried model that have at least one
// related row in C, correlated on the given column pair and narrowed by
// filters:
//
//	types.FilterExists[*Item](ItemCols.SampleID, SampleCols.ID,
//	    ItemCols.Status.Eq(StatusDone))
//	// EXISTS (SELECT 1 FROM `items`
//	//         WHERE `items`.`sample_id` = `samples`.`id`
//	//           AND `items`.`status` = ? AND `items`.`deleted_at` IS NULL)
//
// The shared type parameter T is what makes the correlation safe: two column
// references only satisfy the same ColumnRef[T] when their Go types match, so
// correlating a string column with an integer one does not compile. The two
// table names come from C and from the queried model, so a column reference
// never has to carry a table name.
//
// It is an ordinary Filter, so List, Count, Export and Aggregate all accept
// it; it is service-only and has no URL spelling, because a client-supplied
// subquery is an unbounded read of a table the endpoint never named.
func FilterExists[C Model, T any](child, parent ColumnRef[T], filters ...Filter) Filter {
	return subqueryFilter[C](child, parent, filters, false)
}

// FilterNotExists matches rows that have no related row in C. Note that it is
// not the negation of a filtered FilterExists over the same rows: a row whose
// related rows all fail filters matches, and so does a row with no related
// rows at all.
func FilterNotExists[C Model, T any](child, parent ColumnRef[T], filters ...Filter) Filter {
	return subqueryFilter[C](child, parent, filters, true)
}

// subqueryFilter builds the shared value of both subquery constructors. The
// related model is allocated here rather than at render time so the database
// layer needs no type parameter of its own to reach the child table.
func subqueryFilter[C Model, T any](child, parent ColumnRef[T], filters []Filter, negate bool) Filter {
	sub := Subquery{Filters: filters, Negate: negate}
	if child != nil {
		sub.ChildColumn = child.ColumnName()
	}
	if parent != nil {
		sub.ParentColumn = parent.ColumnName()
	}
	typ := reflect.TypeFor[C]()
	if typ.Kind() == reflect.Pointer {
		if m, ok := reflect.New(typ.Elem()).Interface().(C); ok {
			sub.Model = m
		}
	}
	return Filter{Op: FilterOpExists, Value: sub}
}
