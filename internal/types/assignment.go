package types

// Assignment is one column-value write, the unit UpdateByID accepts. Service
// code builds assignments through the generated column references
// (SampleCols.Status.Set(v)), whose typed front end stops a wrong-typed value
// or a misspelled column at compile time; generic code assigns through a
// reference minted for its type parameter. The Assign constructor takes a
// plain column name for framework code that learns the column only at run
// time. The fields are unexported so an assignment only comes from those
// constructors; Table, Column and Value read it back.
//
// An Assignment never holds SQL. Column names are quoted by the database
// layer and values bind as statement parameters.
type Assignment struct {
	// table is the table the column belongs to: filled in by a column
	// reference, empty from Assign, which names the chain's own model. A
	// write refuses an assignment of another model's column, which may well
	// share the name with one of its own.
	table  string
	column string
	value  any
}

// Table returns the table the assigned column belongs to, or "" when the
// assignment names the chain's own model's column by name alone.
func (a Assignment) Table() string { return a.table }

// Column returns the snake case name of the assigned column.
func (a Assignment) Column() string { return a.column }

// Value returns the value the assignment writes to the column.
func (a Assignment) Value() any { return a.value }

// Assign builds an assignment of value to the named database column of the
// chain's own model.
func Assign(column string, value any) Assignment {
	return Assignment{column: column, value: value}
}

// NewAssignment builds an assignment on a named table. It serves the
// framework's own tests; the public types package does not forward it.
func NewAssignment(table, column string, value any) Assignment {
	return Assignment{table: table, column: column, value: value}
}
