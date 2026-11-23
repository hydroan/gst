package modelschema

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/datatypes"
)

// ValidateColumnTypes refuses a column a model declares by a type the
// framework has its own for: a type of gorm.io/datatypes the framework's
// stand in for (see datatypes.Replacement), and a date column declared by
// any type other than datatypes.Date, a time.Time a gorm type tag declares
// a date among them. Only the framework's types are what the wire carries
// them as: the library's Date and a time.Time write the instant of a
// midnight, 2026-01-02T00:00:00Z, which is that day to a reader in the zone
// it was written in alone, its Time reads a text it cannot read as midnight,
// and no generator knows its JSON types. A model with no such column has
// nothing to hold to.
//
// Every such column is reported in one error, which table preparation fails
// the start with and the column inspection gg builds into a project hands to
// gg check; the public inspect.ValidateColumnTypes forwards to it. For a
// model Record declaring Day as the Date of gorm.io/datatypes, Due
// *time.Time under gorm:"type:date" and Tags as a JSONSlice of the library
// the error reads:
//
//	model Record: field Day is declared gorm.io/datatypes.Date, declare it datatypes.Date; field Due is a date column declared *time.Time, declare it *datatypes.Date; field Tags is declared gorm.io/datatypes.JSONSlice, declare it datatypes.JSONSlice; datatypes is github.com/hydroan/gst/datatypes
func ValidateColumnTypes(typ reflect.Type) error {
	columns, err := Columns(typ)
	if err != nil {
		return err
	}
	var refused []string
	for _, column := range columns {
		declared, pointer := column.Type, ""
		for declared.Kind() == reflect.Pointer {
			declared, pointer = declared.Elem(), "*"
		}
		name, _, _ := strings.Cut(declared.Name(), "[")
		switch own, replaced := datatypes.Replacement(name); {
		case declared.PkgPath() == datatypes.LibraryPath && replaced:
			refused = append(refused, fmt.Sprintf("field %s is declared %s%s.%s, declare it %sdatatypes.%s", column.GoName, pointer, datatypes.LibraryPath, name, pointer, own))
		case IsDateColumn(column) && declared != reflect.TypeFor[datatypes.Date]():
			refused = append(refused, fmt.Sprintf("field %s is a date column declared %s, declare it %sdatatypes.Date", column.GoName, column.Type, pointer))
		}
	}
	if len(refused) == 0 {
		return nil
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return errors.Newf("model %s: %s; datatypes is github.com/hydroan/gst/datatypes", typ.Name(), strings.Join(refused, "; "))
}
