package modelschema

import (
	"reflect"
	"strings"
	"time"

	"gorm.io/datatypes"
	gormschema "gorm.io/gorm/schema"
)

// ColumnClass is the aggregate capability a column carries. It decides which
// reference gg gen writes for the column and which aggregate functions the
// query builder accepts for it at build time, so both consumers read the
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

// dateType is datatypes.Date, the calendar day a date column stores, which
// binds as a time at midnight and reads back as one; see IsDateType.
var dateType = reflect.TypeFor[datatypes.Date]()

// ClassifyColumn reports the aggregate capability of a column, read from the
// data types gorm resolved for it by the rule the filters read too: a column
// IsNumericColumn calls a number carries SUM and AVG, one IsTimeColumn calls
// a time carries the time buckets, and every other column carries the
// aggregates that cannot be silently wrong.
//
// gorm reads a driver.Valuer through the value it holds, so the uuid, JSON
// and enum types stored as text are text to it, and so is a decimal stored
// as a string. Treating those as numeric would let in exactly the failure
// the split exists to prevent: MySQL and SQLite answer SUM over a text column
// with 0 and a warning rather than an error, so the mistake reaches a report
// as a plausible wrong number instead of a failure. SUM or AVG over them is
// therefore refused at build time whichever reference names it, a
// NumericColumn minted by hand included; a decimal that is summed is stored
// in a numeric Go type. A time of day is other too: added up or bucketed, its
// readings make numbers that mean nothing.
func ClassifyColumn(c Column) ColumnClass {
	switch {
	case IsNumericColumn(c):
		return ColumnClassNumeric
	case IsTimeColumn(c):
		return ColumnClassTime
	default:
		return ColumnClassOther
	}
}

// dateDataType is the data type gorm reads for datatypes.Date, the name the
// type declares through GormDataTypeInterface: gorm names no data type of
// its own for a calendar day.
var dateDataType = gormschema.DataType(datatypes.Date{}.GormDataType())

// timeOfDayType is datatypes.Time, the time of day a TIME column stores. It
// declares the time data type an instant carries, and only its
// GormDBDataType, which needs a dialect to answer, tells the column it
// creates apart from an instant's, so the type itself is what tells a time
// of day apart.
var timeOfDayType = reflect.TypeFor[datatypes.Time]()

// IsTimeColumn reports whether a column stores a time by the data type gorm
// reads for its field: an instant — time.Time, or the time sql.NullTime,
// gorm.DeletedAt or a named time type holds — or a calendar day,
// datatypes.Date (see IsDateType). A comparison on such a column compares
// times on every dialect, and a value sent for it is RFC 3339. A time of day
// is not one; see IsTimeOfDayColumn.
func IsTimeColumn(c Column) bool {
	return (c.GORMDataType == gormschema.Time || c.GORMDataType == dateDataType) && !IsTimeOfDayColumn(c)
}

// IsTimeOfDayColumn reports whether a column stores a time of day,
// datatypes.Time, pointers dereferenced. A value sent for it names a time on
// no particular day, in time.TimeOnly.
func IsTimeOfDayColumn(c Column) bool {
	typ := c.Type
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ == timeOfDayType
}

// IsNumericColumn reports whether a column stores a number by the data type
// gorm reads for its field: an integer, signed or not, or a float, the ones
// sql.NullInt64, sql.NullFloat64 and their kin hold included.
func IsNumericColumn(c Column) bool {
	switch c.GORMDataType {
	case gormschema.Int, gormschema.Uint, gormschema.Float:
		return true
	default:
		return false
	}
}

// IsJSONColumn reports whether a column stores a JSON document: its Go type
// declares a JSON data type, the way the gorm.io/datatypes types do, or its
// type tag declares a JSON column. Either data type gorm resolved counts,
// because a type naming its database type itself, as the datatypes family
// does, keeps that column type whatever the tag declares.
func IsJSONColumn(c Column) bool {
	return isJSONDataType(c.GORMDataType) || isJSONDataType(c.DataType)
}

// IsDateType reports whether a column type stores a calendar day, which is
// datatypes.Date, pointers dereferenced. A date column is a time column to
// ClassifyColumn and IsTimeColumn alike; this tells the consumers reading
// values for it apart from an instant's: a value sent for a date column names
// a day, not a time of day, and is read as the UTC day the column stores (see
// UTCDay).
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

// isJSONType reports whether a column type stores as a JSON document. The
// answer comes from the type itself through gorm's GormDataTypeInterface,
// which is how the gorm.io/datatypes types (JSON, JSONType, JSONSlice,
// JSONMap) and custom JSON wrappers declare their column type. Pointers are
// dereferenced, and the method is looked up on both receivers.
//
// It reads the Go type alone, which is all IsJSONStringsType has to go on;
// a column's own answer, a type tag included, is IsJSONColumn's.
func isJSONType(typ reflect.Type) bool {
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
	return isJSONDataType(gormschema.DataType(decl.GormDataType()))
}

// isJSONDataType reports whether a gorm data type names a JSON document. The
// name is matched by its "json" prefix rather than by equality: the
// datatypes family does not spell one name (JSONMap declares "jsonmap", the
// others "json"), and a dialect-flavored type or tag may declare "jsonb".
func isJSONDataType(name gormschema.DataType) bool {
	return strings.HasPrefix(strings.ToLower(string(name)), "json")
}

// IsJSONStringsType reports whether a column type stores a JSON array of
// strings: a JSON document type by isJSONType whose Go shape is a slice of
// strings, datatypes.JSONSlice[string] for one. Pointers are dereferenced.
//
// gg gen writes the JSONStringsColumn reference for such a column, the one
// carrying the membership test: the dialects agree on membership of a string
// element only (PostgreSQL's jsonb ? operator matches no other), so a
// document of another shape gets no reference that offers the test.
func IsJSONStringsType(typ reflect.Type) bool {
	if !isJSONType(typ) {
		return false
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.String
}
