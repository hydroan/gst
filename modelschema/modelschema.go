// Package modelschema exposes the model column resolution used by the
// framework.
//
// It exists for the toolchain, not for application code: gg gen compiles a
// short program inside the project's own module to inspect the registered
// models, and that program cannot reach internal/modelschema, where the
// resolution lives. Forwarding it here lets the generated program resolve
// exactly the columns the framework resolves at runtime, instead of carrying
// a second copy of the rules that would drift from it.
//
// Application code never needs this package: filters name their columns
// through the typed column references gg gen writes next to each model.
// Framework code imports internal/modelschema directly.
package modelschema

import (
	"reflect"

	"github.com/hydroan/gst/internal/modelschema"
)

// Column describes one database column of a model.
type Column = modelschema.Column

// ColumnClass describes the aggregate capability of a column type.
type ColumnClass = modelschema.ColumnClass

// The column classes gg gen maps to the generated column reference types.
const (
	ColumnClassOther   = modelschema.ColumnClassOther
	ColumnClassNumeric = modelschema.ColumnClassNumeric
	ColumnClassTime    = modelschema.ColumnClassTime
)

// Columns returns every database column of a model struct type, sorted by
// database column name.
func Columns(typ reflect.Type) ([]Column, error) {
	return modelschema.Columns(typ)
}

// ClassifyColumn reports the aggregate capability of a column type, which
// decides the column reference gg gen writes for it.
func ClassifyColumn(typ reflect.Type) ColumnClass {
	return modelschema.ClassifyColumn(typ)
}

// ByQueryName indexes columns by the URL parameter name clients filter with.
func ByQueryName(columns []Column) map[string]Column {
	return modelschema.ByQueryName(columns)
}

// ByGoName indexes columns by their Go struct field name.
func ByGoName(columns []Column) map[string]Column {
	return modelschema.ByGoName(columns)
}

// QueryColumnName resolves the URL parameter name of a struct field.
func QueryColumnName(field reflect.StructField) string {
	return modelschema.QueryColumnName(field)
}
