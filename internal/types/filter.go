package types

// Filter is one field-level filter to apply as an AND condition: the column it
// compares, the table that column belongs to, the operator and the value.
//
// The fields are unexported so a filter only comes from the constructors that
// keep it well formed: the generated column references, a reference minted
// for a type parameter in generic code, the grouping, subquery and constant
// constructors, and the plain-name constructors the framework parses requests
// with. Table, Column, Op and Value read a filter back; Split, Values,
// ExcludedValues and Bounds on a column reference read one column's filters
// converted to the column's type.
//
// Table is filled in by a column reference and left empty by the plain-name
// constructors, which names the queried model's own table. A filter carrying
// another table is applied to that table when the query joins it and fails
// closed otherwise. The value is always bound as a statement parameter, in the
// shape its operator requires:
//
//   - FilterOpIn and FilterOpNotIn require a slice or array value.
//   - FilterOpIsNull requires a bool value.
//   - FilterOpLike, FilterOpNotLike, FilterOpStartsWith, FilterOpEndsWith,
//     FilterOpRegex, FilterOpNotRegex, and FilterOpJSONContains require a
//     string value.
//   - FilterOpOr and FilterOpAnd require a non-empty []Filter value and carry
//     no column: they group their children instead of naming one themselves.
//   - FilterOpExists requires a Subquery value and carries no column; see
//     FilterExists.
//   - FilterOpEqCol requires the other column: its name as a string, or the
//     column reference itself, which also carries its table. It renders
//     inside a subquery and inside a join; see FilterEqCol.
//   - FilterOpFalse carries neither column nor value; see FilterFalse.
//   - The comparison operators take a scalar value (string, numeric,
//     time.Time); slices, arrays, and nil are rejected.
//
// A value that violates these rules fails closed in the database layer.
type Filter struct {
	table  string
	column string
	op     FilterOp
	value  any
}

// Table returns the table the filter's column belongs to, or "" when the
// filter names the queried model's own column or no column at all.
func (f Filter) Table() string { return f.table }

// Column returns the snake case name of the column the filter compares, or ""
// for a group, a subquery and the constant predicate, which name no column.
func (f Filter) Column() string { return f.column }

// Op returns the filter's operator.
func (f Filter) Op() FilterOp { return f.op }

// Value returns the filter's value in the shape its operator requires; see
// Filter. A filter parsed from a request carries the value the parser
// normalized: the string spelling of a number, the UTC wall clock in
// FilterTimeLayout for a time, and a []string for the members of in and notin.
// Values, ExcludedValues and Bounds on a column reference convert it to the
// column's type. A slice, group or subquery value is shared with the filter
// and must not be modified.
func (f Filter) Value() any { return f.value }

// NewFilter builds a filter from its parts, without the shape checks the
// other constructors lock in at compile time. It serves the framework's own
// tests, which exercise how the database layer fails closed on a malformed
// filter; the public gst package does not forward it.
func NewFilter(table, column string, op FilterOp, value any) Filter {
	return Filter{table: table, column: column, op: op, value: value}
}

// FilterTimeLayout is the canonical layout a time-typed filter value parsed
// from a URL is normalized to. The value travels as a string rather than a
// time.Time on purpose: binding a time.Time would let the driver re-render it
// in its own location, while the string pins the wall-clock time the parser
// resolved. The pinned wall clock is UTC, the one wall clock the framework
// stores on every dialect.
//
// The URL parser writes the layout and Values, ExcludedValues and Bounds on a
// column reference read it back, so a service reading a time goes through
// them rather than parsing with the layout directly.
const FilterTimeLayout = "2006-01-02 15:04:05.999999999"

// The Filter constructors below build one Filter per operator from a plain
// column name. They serve code that learns the column only at run time, such
// as framework internals reading it from a request; code on a concrete model
// reaches the same operators through the generated column references, and
// generic code through a reference minted for its type parameter, see Column.
// The grouping, subquery and constant constructors have no column-reference
// form and serve every caller. Each signature locks the value shape its operator
// expects, so a malformed filter cannot be expressed without bypassing the
// constructors. Column is a snake case column name; validating it against the
// model's queryable columns remains the caller's responsibility.

// FilterEq matches rows where column equals value.
func FilterEq(column string, value any) Filter {
	return Filter{column: column, op: FilterOpEq, value: value}
}

// FilterNe matches rows where column does not equal value.
func FilterNe(column string, value any) Filter {
	return Filter{column: column, op: FilterOpNe, value: value}
}

// FilterGt matches rows where column is greater than value.
func FilterGt(column string, value any) Filter {
	return Filter{column: column, op: FilterOpGt, value: value}
}

// FilterGte matches rows where column is greater than or equal to value.
func FilterGte(column string, value any) Filter {
	return Filter{column: column, op: FilterOpGte, value: value}
}

// FilterLt matches rows where column is less than value.
func FilterLt(column string, value any) Filter {
	return Filter{column: column, op: FilterOpLt, value: value}
}

// FilterLte matches rows where column is less than or equal to value.
func FilterLte(column string, value any) Filter {
	return Filter{column: column, op: FilterOpLte, value: value}
}

// FilterIn matches rows where column is one of values. The slice is bound as
// a whole; an empty slice matches nothing.
func FilterIn[T any](column string, values []T) Filter {
	return Filter{column: column, op: FilterOpIn, value: append([]T(nil), values...)}
}

// FilterNotIn matches rows where column is none of values. The slice is
// bound as a whole; an empty slice matches nothing (SQL NOT IN over an empty
// list never holds), it does not mean "exclude nothing".
func FilterNotIn[T any](column string, values []T) Filter {
	return Filter{column: column, op: FilterOpNotIn, value: append([]T(nil), values...)}
}

// FilterLike matches rows where column contains value as a substring; value
// is escaped and matches literally.
func FilterLike(column, value string) Filter {
	return Filter{column: column, op: FilterOpLike, value: value}
}

// FilterNotLike matches rows where column does not contain value as a
// substring; value is escaped and matches literally.
func FilterNotLike(column, value string) Filter {
	return Filter{column: column, op: FilterOpNotLike, value: value}
}

// FilterStartsWith matches rows where column starts with value; value is
// escaped and matches literally, and the prefix form can use an index.
func FilterStartsWith(column, value string) Filter {
	return Filter{column: column, op: FilterOpStartsWith, value: value}
}

// FilterEndsWith matches rows where column ends with value; value is escaped
// and matches literally.
func FilterEndsWith(column, value string) Filter {
	return Filter{column: column, op: FilterOpEndsWith, value: value}
}

// FilterIsNull matches rows whose column is NULL.
func FilterIsNull(column string) Filter {
	return Filter{column: column, op: FilterOpIsNull, value: true}
}

// FilterIsNotNull matches rows whose column is not NULL.
func FilterIsNotNull(column string) Filter {
	return Filter{column: column, op: FilterOpIsNull, value: false}
}

// FilterRegex matches rows where column matches the regular expression expr
// (dialect-aware REGEXP).
func FilterRegex(column, expr string) Filter {
	return Filter{column: column, op: FilterOpRegex, value: expr}
}

// FilterNotRegex matches rows where column does not match the regular
// expression expr.
func FilterNotRegex(column, expr string) Filter {
	return Filter{column: column, op: FilterOpNotRegex, value: expr}
}

// FilterJSONContains matches rows whose JSON array column contains value as
// a member.
func FilterJSONContains(column, value string) Filter {
	return Filter{column: column, op: FilterOpJSONContains, value: value}
}

// FilterFalse matches nothing. It is the condition a permission hook returns
// when the caller may see no row at all. Unlike an empty filter list it is a
// real condition, so it disables the empty-query safety check; unlike a
// filter the renderer cannot apply it is deliberate, so nothing is logged. It
// renders as 1 = 0 on every dialect and composes like any other filter,
// inside groups, subqueries and conditional measures included.
func FilterFalse() Filter {
	return Filter{op: FilterOpFalse}
}

// FilterOr groups filters that are OR-combined with each other. The group as a
// whole stays AND-combined with every other condition of the query, so a
// mandatory condition such as tenant scoping can never be absorbed into the
// alternatives:
//
//	Filters: []types.Filter{
//	    SampleCols.TenantID.Eq(tenant),
//	    types.FilterOr(
//	        SampleCols.Name.Like(keyword),
//	        SampleCols.Code.Like(keyword),
//	    ),
//	}
//	// WHERE tenant_id = ? AND (name LIKE ? OR code LIKE ?)
//
// Children may themselves be groups, which is how nesting is expressed; see
// FilterAnd for the "(a AND b) OR (c AND d)" shape. A group with no children
// fails closed.
func FilterOr(filters ...Filter) Filter {
	return Filter{op: FilterOpOr, value: append([]Filter(nil), filters...)}
}

// FilterAnd groups filters that are AND-combined with each other. Filters are
// already AND-combined at the top level, so the group exists to nest an AND
// inside an OR group:
//
//	Filters: []types.Filter{
//	    SampleCols.TenantID.Eq(tenant),
//	    types.FilterOr(
//	        types.FilterAnd(
//	            SampleCols.Kind.Eq(KindPrimary),
//	            SampleCols.Status.Eq(StatusDone),
//	        ),
//	        types.FilterAnd(
//	            SampleCols.Kind.Eq(KindSecondary),
//	            SampleCols.Status.Eq(StatusPending),
//	        ),
//	    ),
//	}
//	// WHERE tenant_id = ?
//	//   AND ((kind = ? AND status = ?) OR (kind = ? AND status = ?))
//
// A group with no children fails closed.
func FilterAnd(filters ...Filter) Filter {
	return Filter{op: FilterOpAnd, value: append([]Filter(nil), filters...)}
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
//
// The string form names the columns alone, which a subquery can place because
// both of its tables are known; a join needs the tables too, so its predicates
// are written with Column.EqCol, the typed front end that keeps the two
// columns of the same Go type and carries both tables.
func FilterEqCol(column, parent string) Filter {
	return Filter{column: column, op: FilterOpEqCol, value: parent}
}
