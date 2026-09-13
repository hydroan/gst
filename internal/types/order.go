package types

// OrderDirection is the sort direction of one ORDER BY term. The set is
// closed, so a direction can never carry SQL the way a free-form order string
// could.
type OrderDirection string

const (
	// OrderAsc sorts ascending. The zero OrderDirection is also read as
	// ascending, matching SQL's own default for an ORDER BY term.
	OrderAsc OrderDirection = "ASC"
	// OrderDesc sorts descending.
	OrderDesc OrderDirection = "DESC"
)

// Valid reports whether the direction is one this package defines. A value
// from outside the set would otherwise fall through to ascending and sort a
// report the opposite of what the caller asked for.
func (d OrderDirection) Valid() bool {
	switch d {
	case "", OrderAsc, OrderDesc:
		return true
	default:
		return false
	}
}

// Flip returns the opposite direction. Cursor pagination uses it to read a
// feed backwards: traveling against the feed reverses both the boundary
// comparison and the ORDER BY.
func (d OrderDirection) Flip() OrderDirection {
	if d == OrderDesc {
		return OrderAsc
	}
	return OrderDesc
}

// Order is one ORDER BY term: a column and the direction to sort it by.
//
// The fields are unexported so an order only comes from the constructors:
// the generated column references (SampleCols.CreatedAt.Desc()), which cannot
// name a column the model does not have, a reference minted for a type
// parameter in generic code, and the Asc and Desc constructors, which take a
// plain column name for code that learns the column only at run time:
// framework internals and URL parsing. Table, Column and Descending read an
// order back, and SortsBy tells whether it sorts by a given column.
//
// Table is filled in by a column reference and left empty by Asc and Desc.
// The chain's reads and a select check it: a select that joins tells two
// tables' columns of one name apart by it, and a chain refuses an order of
// another model. A union orders its result columns by name and reads no table,
// and WithExpand orders the associated table, so neither checks it. An Order
// with an empty column is skipped rather than rendered.
type Order struct {
	table     string
	column    string
	direction OrderDirection
}

// Table returns the table the order's column belongs to, or "" when the order
// names the queried model's column by name alone.
func (o Order) Table() string { return o.table }

// Column returns the snake case name of the column the order sorts by.
func (o Order) Column() string { return o.column }

// SortsBy reports whether the order sorts by column. An order parsed from a
// request names no table and matches by column name alone; an order built from
// a column reference names its table, which has to be the column's table too.
func (o Order) SortsBy(column AnyColumnRef) bool {
	return namesColumn(o.table, o.column, column)
}

// Descending reports whether the order sorts descending. Any other direction,
// the zero one included, sorts ascending.
func (o Order) Descending() bool { return o.direction == OrderDesc }

// Asc builds an ascending order term for column.
func Asc(column string) Order { return Order{column: column, direction: OrderAsc} }

// Desc builds a descending order term for column.
func Desc(column string) Order { return Order{column: column, direction: OrderDesc} }

// NewOrder builds an order from its parts. It serves the framework's own code
// and tests, which need an order on a named table or with a direction outside
// the set; the public types package does not forward it.
func NewOrder(table, column string, direction OrderDirection) Order {
	return Order{table: table, column: column, direction: direction}
}

// OrderDirectionOf returns the direction an order sorts in, for the database
// layer, which validates the direction and flips it for a backward cursor
// read; the public types package does not forward it.
func OrderDirectionOf(o Order) OrderDirection { return o.direction }

// Ordering is what the OrderBy methods of a select, a window and a union
// accept: an Order sorting by a column reference, or a TermOrder sorting by a
// projection term. The set is closed, so an ordering can never carry SQL the
// way a free-form string could.
type Ordering interface {
	sealedOrdering()
}

func (Order) sealedOrdering() {}
