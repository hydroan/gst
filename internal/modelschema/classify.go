package modelschema

import (
	"reflect"
	"strings"
	"time"

	"gorm.io/datatypes"
	gormschema "gorm.io/gorm/schema"
)

// ColumnClass is the aggregate capability a column type carries. It decides
// which reference gg gen writes for the column and which aggregate functions
// the query builder accepts for it at build time, so both consumers read the
// same rule rather than each carrying a copy that could drift.
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

// timeType and dateType are the two column types holding a time value, the
// ones that get time bucketing: time.Time, an instant, and datatypes.Date,
// the calendar day a date column stores, which binds as a time at midnight
// and reads back as one. A named type whose underlying type is time.Time is
// neither: the framework knows how these two bind and read back, and nothing
// of another type.
var (
	timeType = reflect.TypeFor[time.Time]()
	dateType = reflect.TypeFor[datatypes.Date]()
)

// ClassifyColumn reports the aggregate capability of a column type. Pointers
// are dereferenced, since an aggregate reads the pointed-to value.
//
// Classification reads reflect.Kind only. Recognizing decimal types through
// driver.Valuer looks tempting, but uuid, JSON and enum types stored as text
// implement it too, and treating those as numeric would let in exactly the
// failure the split exists to prevent: MySQL and SQLite answer SUM over a
// text column with 0 and a warning rather than an error, so the mistake
// reaches a report as a plausible wrong number instead of a failure. A decimal
// stored as a struct is therefore classified as other, and SUM or AVG over it
// is refused at build time whichever reference names it, a NumericColumn
// minted by hand included; a decimal that is summed is stored in a numeric Go
// type.
func ClassifyColumn(typ reflect.Type) ColumnClass {
	if typ == nil {
		return ColumnClassOther
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == timeType || typ == dateType {
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

// IsDateType reports whether a column type stores a calendar day, which is
// datatypes.Date, pointers dereferenced. A date column is a time column to
// ClassifyColumn; this tells the consumers reading values for it apart from
// an instant's: a value sent for a date column names a day, not a time of
// day, and is read as the UTC day the column stores (see UTCDay).
func IsDateType(typ reflect.Type) bool {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ == dateType
}

// UTCDay returns the calendar day t falls on in UTC, the one time base of
// the framework, at midnight: the value a date column stores for t and the
// value a date filter compares by, so that a day a client spells with its
// own offset, 2026-01-02T00:00:00+08:00, is the same day, 2026-01-01, on
// every transport and in every database. The zero time stays zero.
func UTCDay(t time.Time) time.Time {
	year, month, day := t.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
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
	decl, ok := reflect.TypeAssert[gormschema.GormDataTypeInterface](value.Elem())
	if !ok {
		if decl, ok = reflect.TypeAssert[gormschema.GormDataTypeInterface](value); !ok {
			return false
		}
	}
	return strings.HasPrefix(strings.ToLower(decl.GormDataType()), "json")
}
