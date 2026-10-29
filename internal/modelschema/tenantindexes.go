package modelschema

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
)

// ValidateTenantIndexes refuses a unique index that a model embedding
// tenant.Scope declares without the tenant among its fields. The rows of such
// a model belong to one tenant each and every statement reaches one tenant's
// rows, so a value unique across the whole table is one tenant refusing what
// another holds, with no row in sight to explain the refusal: uniqueness is
// declared within the tenant, TenantID among the fields. A model without the
// tenant column, and an index that is not unique, have nothing to hold to.
//
// Every such index is reported in one error, which table preparation fails
// the start with and the column inspection gg builds into a project hands to
// gg check; the public inspect.ValidateTenantIndexes forwards to it. For a
// model Record declaring {Fields: []string{"Code"}, Unique: true} the error
// reads:
//
//	model Record: unique index (Code) leaves out TenantID; a model embedding tenant.Scope is unique within a tenant, so every unique index names TenantID among its fields
func ValidateTenantIndexes(typ reflect.Type) error {
	columns, err := Columns(typ)
	if err != nil {
		return err
	}
	tenantField := ""
	for _, column := range columns {
		if column.Type == tenantColumnType {
			tenantField = column.GoName
		}
	}
	if tenantField == "" {
		return nil
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	var leftOut []string
	for _, decl := range IndexDeclarations(reflect.New(typ).Interface()) {
		if decl.Unique && !slices.Contains(decl.Fields, tenantField) {
			leftOut = append(leftOut, fmt.Sprintf("unique index (%s) leaves out %s", strings.Join(decl.Fields, ", "), tenantField))
		}
	}
	if len(leftOut) == 0 {
		return nil
	}
	return errors.Newf("model %s: %s; a model embedding tenant.Scope is unique within a tenant, so every unique index names %s among its fields", typ.Name(), strings.Join(leftOut, "; "), tenantField)
}
