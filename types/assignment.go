package types

// Assignment is one column-value write, the unit UpdateByID accepts. Service
// code should build assignments through the generated column references
// (SampleCols.Status.Set(v)), whose typed front end stops a wrong-typed value
// or a misspelled column at compile time. The Assign constructor takes a
// plain column name and exists for code that cannot reference a concrete
// model: framework internals and dynamic column loops.
//
// An Assignment never holds SQL. Column names are quoted by the database
// layer and values bind as statement parameters.
type Assignment struct {
	// Table is the table the column belongs to: filled in by a column
	// reference, empty from Assign, which names the chain's own model. A
	// write refuses an assignment of another model's column, which may well
	// share the name with one of its own.
	Table  string
	Column string
	Value  any
}

// Assign builds an assignment of value to the named database column of the
// chain's own model.
func Assign(column string, value any) Assignment {
	return Assignment{Column: column, Value: value}
}
