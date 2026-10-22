package modelschema

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/tenant"
)

// fixedColumns maps the Go name of each field model.Base and model.AutoBase
// declare to the column the framework names it by.
var fixedColumns = map[string]string{
	"ID":        "id",
	"CreatedBy": "created_by",
	"UpdatedBy": "updated_by",
	"CreatedAt": "created_at",
	"UpdatedAt": "updated_at",
	"DeletedAt": "deleted_at",
}

// fixedTenantColumn is the column the framework names the tenant by, the one
// tenant.Scope declares.
const fixedTenantColumn = "tenant_id"

// tenantColumnType is the type that tells the tenant column apart.
var tenantColumnType = reflect.TypeFor[tenant.ID]()

// ValidateFrameworkColumns refuses a model mapping a framework field to a
// column other than the one the framework fixes for it. The framework names
// those columns itself instead of resolving them for each model, id for the
// record a route names and deleted_at for soft deletion among them, so a
// model renaming one would have the framework work on a column other than
// the one the model's field maps to. The fields of model.Base and
// model.AutoBase are told by their Go name, so a field the model re-declares
// under one of those names is held to the same column, while a field of a
// struct embedded by name, Audit.CreatedAt, is not one of them. The tenant
// column is told by its type, which tenant.Scope alone carries, so a field
// of the model's own named TenantID stays the model's. A model renames a
// framework column through a column tag on the field it re-declares, or by
// embedding tenant.Scope under a prefix.
//
// Every renamed column is reported in one error, which table preparation
// fails the start with and the column inspection gg builds into a project
// hands to gg check; the public modelschema.ValidateFrameworkColumns forwards
// to it. For a model Record re-declaring the soft-delete field under
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
		if column.Type == tenantColumnType {
			fixed, framework = fixedTenantColumn, true
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
