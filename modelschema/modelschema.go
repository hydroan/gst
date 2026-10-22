// Package modelschema exposes the model column resolution used by the
// framework.
//
// It exists for the toolchain, not for application code: gg gen compiles a
// short program inside the project's own module to inspect the project's
// models, and that program cannot reach the internal packages where the
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

	"github.com/hydroan/gst/internal/modelregistry"
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

// ValidateFrameworkColumns refuses a model mapping a framework field, one of
// model.Base, model.AutoBase or tenant.Scope, to a column other than the one
// the framework fixes for it. The inspection program asks it of every model
// it inspects, so gg check refuses what table preparation refuses at startup.
func ValidateFrameworkColumns(typ reflect.Type) error {
	return modelschema.ValidateFrameworkColumns(typ)
}

// ClassifyColumn reports the aggregate capability of a column, which decides
// the column reference gg gen writes for it.
func ClassifyColumn(c Column) ColumnClass {
	return modelschema.ClassifyColumn(c)
}

// IsJSONStringsType reports whether a column type stores a JSON array of
// strings, which decides whether gg gen writes the JSONStringsColumn
// reference for it.
func IsJSONStringsType(typ reflect.Type) bool {
	return modelschema.IsJSONStringsType(typ)
}

// IsQueryable reports whether m opted in to framework query parameters by
// embedding model.Query. The inspection program uses it to keep only the
// unregistered models that own a filter and sort column namespace; the
// judgment stays sealed in internal/modelregistry, forwarded here for the
// same reason column resolution is.
func IsQueryable(m any) bool {
	return modelregistry.IsQueryable(m)
}
