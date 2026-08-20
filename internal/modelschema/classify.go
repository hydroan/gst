package modelschema

import (
	"reflect"
	"strings"
	"time"

	"gorm.io/gorm/schema"
)

// ColumnClass is the aggregate capability a column type carries. It decides
// which reference gg gen writes for the column and which aggregate functions
// the string-name constructors accept for it, so both consumers read the same
// rule rather than each carrying a copy that could drift.
type ColumnClass int

const (
	// ColumnClassOther carries the aggregate functions that cannot be silently
	// wrong on any type: COUNT, COUNT DISTINCT, MIN and MAX.
	ColumnClassOther ColumnClass = iota
	// ColumnClassNumeric additionally carries SUM and AVG.
	ColumnClassNumeric
	// ColumnClassTime additionally carries time bucketing.
	ColumnClassTime
)

// timeType is the one type that gets time bucketing. A named type whose
// underlying type is time.Time is deliberately excluded: it is not a
// time.Time, so a reference typed on it would not compile.
var timeType = reflect.TypeFor[time.Time]()

// ClassifyColumn reports the aggregate capability of a column type. Pointers
// are dereferenced, since an aggregate reads the pointed-to value.
//
// Classification reads reflect.Kind only. Recognizing decimal types through
// driver.Valuer looks tempting, but uuid, JSON and enum types stored as text
// implement it too, and treating those as numeric would reintroduce exactly
// the failure the split exists to prevent: MySQL and SQLite answer SUM over a
// text column with 0 and a warning rather than an error, so the mistake
// reaches a report as a plausible wrong number instead of a failure. A decimal
// stored as a struct is therefore classified as other, and is summed through
// SumOf, whose column is checked against the model schema at build time.
func ClassifyColumn(typ reflect.Type) ColumnClass {
	if typ == nil {
		return ColumnClassOther
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == timeType {
		return ColumnClassTime
	}
	switch typ.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return ColumnClassNumeric
	default:
		return ColumnClassOther
	}
}

// IsJSONType reports whether a column type stores as a JSON document. The
// answer comes from the type itself through gorm's GormDataTypeInterface,
// which is how the gorm.io/datatypes types (JSON, JSONType, JSONSlice,
// JSONMap) and custom JSON wrappers declare their column type. Pointers are
// dereferenced, and the method is looked up on both receivers.
//
// The declared name is matched by its "json" prefix rather than by equality:
// the datatypes family does not spell one name (JSONMap declares "jsonmap",
// the others "json"), and a dialect-flavored wrapper may declare "jsonb".
//
// Consumers use it to keep text operators away from JSON columns where a
// dialect is strict about operand types; see the WithQuery JSON handling in
// the database package.
func IsJSONType(typ reflect.Type) bool {
	if typ == nil {
		return false
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	value := reflect.New(typ)
	decl, ok := reflect.TypeAssert[schema.GormDataTypeInterface](value.Elem())
	if !ok {
		if decl, ok = reflect.TypeAssert[schema.GormDataTypeInterface](value); !ok {
			return false
		}
	}
	return strings.HasPrefix(strings.ToLower(decl.GormDataType()), "json")
}
