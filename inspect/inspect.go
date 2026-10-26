// Package inspect forwards the framework's model rules to the program gg
// builds to inspect a project's models.
//
// It exists for the toolchain, not for application code: gg gen and gg check
// compile a short program inside the project's own module to inspect the
// project's models, and that program cannot reach the internal packages the
// rules live in. This package forwards the ones it asks: the column
// resolution and the framework column check of internal/modelschema, the
// query name check of internal/urlquery and the query opt-in of
// internal/modelregistry. The program therefore resolves and refuses exactly
// what the framework resolves and refuses at runtime, instead of carrying a
// second copy of the rules that would drift from it.
//
// Application code never needs this package: filters name their columns
// through the typed column references gg gen writes next to each model.
// Framework code imports the internal packages directly.
package inspect

import (
	"reflect"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/urlquery"
)

// Column describes one database column of a model.
type Column = modelschema.Column

// ColumnClass describes the aggregate capability of a column type.
type ColumnClass = modelschema.ColumnClass

// The column classes gg gen writes a numeric or a time column reference for;
// a column of any other class takes the plain one.
const (
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

// ValidateQueryNames refuses a model whose query tags name its URL query
// parameters ambiguously: a name two fields declare, a business field's name
// in the "_" namespace of the framework's own parameters, or a tag holding
// "[", "]" or ".". The inspection program asks it of every model it inspects,
// so gg check refuses what table preparation refuses at startup.
func ValidateQueryNames(typ reflect.Type) error {
	return urlquery.ValidateQueryNames(typ)
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
