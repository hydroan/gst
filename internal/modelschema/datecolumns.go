package modelschema

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/datatypes"
)

// ValidateDateColumns refuses a date column a model declares by a type other
// than datatypes.Date, gst.Date to a project: the Date of gorm.io/datatypes,
// and a time.Time, or any other type, a gorm type tag declares a date. Only
// the framework's Date is a day on the wire:
// the library's type and a time.Time write the instant of a midnight,
// 2026-01-02T00:00:00Z, which is that day to a reader in the zone it was
// written in alone. A model with no date column has nothing to hold to.
//
// Every such column is reported in one error, which table preparation fails
// the start with and the column inspection gg builds into a project hands to
// gg check; the public inspect.ValidateDateColumns forwards to it. For a
// model Record declaring Day as the Date of gorm.io/datatypes and Due
// *time.Time under gorm:"type:date" the error reads:
//
//	model Record: field Day stores a date as datatypes.Date; field Due stores a date as *time.Time; a date column is declared gst.Date, or *gst.Date when it is optional
func ValidateDateColumns(typ reflect.Type) error {
	columns, err := Columns(typ)
	if err != nil {
		return err
	}
	var refused []string
	for _, column := range columns {
		declared := column.Type
		for declared.Kind() == reflect.Pointer {
			declared = declared.Elem()
		}
		if IsDateColumn(column) && declared != reflect.TypeFor[datatypes.Date]() {
			refused = append(refused, fmt.Sprintf("field %s stores a date as %s", column.GoName, column.Type))
		}
	}
	if len(refused) == 0 {
		return nil
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return errors.Newf("model %s: %s; a date column is declared gst.Date, or *gst.Date when it is optional", typ.Name(), strings.Join(refused, "; "))
}
