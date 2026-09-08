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
// Column must already be validated against the model's queryable columns by
// the producer (the List controller validates URL input; service code passing
// orders directly carries the same responsibility). Table is the table the
// column belongs to, filled in by a column reference and empty from the Asc
// and Desc constructors. The chain's reads and a select check it: a select
// that joins tells two tables' columns of one name apart by it, and a chain
// refuses an order of another model. A union orders its result columns by
// name and reads no table, and WithExpand orders the associated table, so
// neither checks it. An Order with an empty column is skipped rather than
// rendered.
//
// Service code should build orders through the generated column references
// (SampleCols.CreatedAt.Desc()), which cannot name a column the model does not
// have. The Asc and Desc constructors take a plain column name and exist for
// code that cannot reference a concrete model: generic helpers, framework
// internals, and URL parsing.
type Order struct {
	Table     string
	Column    string
	Direction OrderDirection
}

// Asc builds an ascending order term for column.
func Asc(column string) Order { return Order{Column: column, Direction: OrderAsc} }

// Desc builds a descending order term for column.
func Desc(column string) Order { return Order{Column: column, Direction: OrderDesc} }

// Ordering is what the OrderBy methods of a select, a window and a union
// accept: an Order sorting by a column reference, or a TermOrder sorting by a
// projection term. The set is closed, so an ordering can never carry SQL the
// way a free-form string could.
type Ordering interface {
	sealedOrdering()
}

func (Order) sealedOrdering() {}
