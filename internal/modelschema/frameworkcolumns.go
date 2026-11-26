package modelschema

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"
)

// fixedColumns maps the Go name of each field model.Base and model.AutoBase
// declare to the column the framework names it by.
var fixedColumns = map[string]string{
	"ID":        IDColumn,
	"CreatedBy": "created_by",
	"UpdatedBy": "updated_by",
	"CreatedAt": "created_at",
	"UpdatedAt": "updated_at",
	"DeletedAt": softDeleteColumn,
}

// IDColumn is the column the framework names the primary key of model.Base
// and model.AutoBase by. ValidateFrameworkColumns holds every model to it, so
// the framework reads a record by its id through this column.
const IDColumn = "id"

// softDeleteColumn is the column the framework names the soft-delete time of
// model.Base and model.AutoBase by: NULL in a live row, the time of the
// delete in a soft-deleted one. ValidateFrameworkColumns holds every model to
// it, so a unique index tells the live rows by this column (see
// IndexPlan.LiveOnly).
const softDeleteColumn = "deleted_at"

// fixedColumnTypes maps the Go type that tells a framework column apart to
// the column the framework names it by, for the columns a package above this
// one declares (see RegisterFixedColumn).
var fixedColumnTypes = make(map[reflect.Type]string)

// RegisterFixedColumn declares that a column of the Go type typ is a
// framework column named column, whatever field of a model holds it, for
// ValidateFrameworkColumns to hold every model to that name. A package whose
// type carries such a column, the tenant of tenant.Scope for one, calls it
// while it initializes, before any model is validated.
func RegisterFixedColumn(typ reflect.Type, column string) {
	fixedColumnTypes[typ] = column
}

// DefaultCursorColumn is the database column cursor pagination falls back to
// when the caller did not name one. It is the primary key of every framework
// base model, the one column a cursor can always page by: no two rows share
// it, so no row falls between two pages, which is also why the database layer
// completes the order of a select and of a window with it.
//
// Rising, not monotonic across writers: model.Base draws its id before the
// insert and model.AutoBase takes it from the sequence at insert time, so of
// two rows written at once the one with the smaller id may commit second. A
// feed read while those writes are in flight can therefore pass over the
// later commit; a feed of rows already committed reads them all.
//
// The database layer applies the fallback when building the boundary
// comparison, and URL parsing resolves the same column to type-check the
// cursor value, so every consumer reads this single definition.
const DefaultCursorColumn = IDColumn

// ValidateFrameworkColumns refuses a model mapping a framework field to a
// column other than the one the framework fixes for it. The framework names
// those columns itself instead of resolving them for each model, id for the
// record a route names and deleted_at for soft deletion among them, so a
// model renaming one would have the framework work on a column other than
// the one the model's field maps to. The fields of model.Base and
// model.AutoBase are told by their Go name, so a field the model re-declares
// under one of those names is held to the same column, while a field of a
// struct embedded by name, Audit.CreatedAt, is not one of them. A column a
// package registered by its type (see RegisterFixedColumn), the tenant column
// of tenant.Scope, is told by that type, which the struct declaring it alone
// carries, so a field of the model's own named TenantID stays the model's. A
// model renames a framework column through a column tag on the field it
// re-declares, or by embedding such a struct under a prefix.
//
// Every renamed column is reported in one error, which table preparation
// fails the start with and the column inspection gg builds into a project
// hands to gg check; the public inspect.ValidateFrameworkColumns forwards to
// it. For a model Record re-declaring the soft-delete field under
// gorm:"column:removed_at" the error reads:
//
//	model Record: framework field DeletedAt maps to column "removed_at" instead of "deleted_at"; the framework fixes the names of its columns, so drop the column tag or embedded prefix renaming them
func ValidateFrameworkColumns(typ reflect.Type) error {
	columns, err := Columns(typ)
	if err != nil {
		return err
	}
	var renamed []string
	for _, column := range columns {
		fixed, framework := fixedColumns[column.GoName]
		if byType, ok := fixedColumnTypes[column.Type]; ok {
			fixed, framework = byType, true
		}
		if framework && column.DBName != fixed {
			renamed = append(renamed, fmt.Sprintf("framework field %s maps to column %q instead of %q", column.GoName, column.DBName, fixed))
		}
	}
	if len(renamed) == 0 {
		return nil
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return errors.Newf("model %s: %s; the framework fixes the names of its columns, so drop the column tag or embedded prefix renaming them", typ.Name(), strings.Join(renamed, "; "))
}
