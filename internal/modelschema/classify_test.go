package modelschema_test

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/modelschema"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// textValuer stands for the uuid, JSON and enum types that are stored as text
// and implement driver.Valuer.
type textValuer struct{ raw string }

func (v textValuer) Value() (driver.Value, error) { return v.raw, nil }

func TestClassifyColumn(t *testing.T) {
	type namedAmount int64
	type namedLabel string
	type namedTime time.Time
	type classified struct {
		ID string `gorm:"primaryKey"`

		Int      int
		Int8     int8
		Int16    int16
		Int32    int32
		Int64    int64
		Uint     uint
		Uint8    uint8
		Uint16   uint16
		Uint32   uint32
		Uint64   uint64
		Float32  float32
		Float64  float64
		Amount   namedAmount
		Optional *int64
		Tally    sql.NullInt64

		At      time.Time
		Closed  *time.Time
		Day     datatypes.Date
		Due     *datatypes.Date
		Named   namedTime
		Settled sql.NullTime
		Removed gorm.DeletedAt

		Note   string
		Flag   bool
		Label  namedLabel
		Digest []byte
		Text   textValuer
		Clock  datatypes.Time
		Doc    datatypes.JSON
	}
	columns, err := modelschema.Columns(reflect.TypeFor[classified]())
	require.NoError(t, err)
	classes := make(map[modelschema.ColumnClass][]string)
	for _, c := range columns {
		class := modelschema.ClassifyColumn(c)
		classes[class] = append(classes[class], c.DBName)
	}

	require.ElementsMatch(t, []string{
		"int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "amount", "optional", "tally",
	}, classes[modelschema.ColumnClassNumeric],
		"every integer and float, named or behind a pointer, and the number sql.NullInt64 holds")
	require.ElementsMatch(t, []string{"at", "closed", "day", "due", "named", "settled", "removed"}, classes[modelschema.ColumnClassTime],
		"an instant and a calendar day, behind a pointer too, and the times a named time type, sql.NullTime and gorm.DeletedAt hold")
	// gorm reads a driver.Valuer through the value it holds, so a type stored
	// as text through one is no number: MySQL and SQLite answer SUM over text
	// with 0 rather than an error, and the mistake would reach a report as a
	// wrong number. A time of day is no instant, and sums to one just as
	// wrong.
	require.ElementsMatch(t, []string{"id", "note", "flag", "label", "digest", "text", "clock", "doc"}, classes[modelschema.ColumnClassOther],
		"text, a flag, bytes, a type stored as text through driver.Valuer, a time of day and a JSON document")
	require.Equal(t, modelschema.ColumnClassOther, modelschema.ClassifyColumn(modelschema.Column{}))
}

// sampleStoredRecord carries a column of each type the column predicates
// tell apart by the data types gorm reads for its field, beside its Go shape:
// values held in Valuer wrappers, column types a type tag declares, a JSON
// type tagged otherwise, a serializer storing text, and a time of day gorm
// gives the time data type of an instant.
type sampleStoredRecord struct {
	ID        string    `gorm:"primaryKey"`
	At        time.Time `gorm:"type:datetime(6)"`
	Settled   sql.NullTime
	Removed   gorm.DeletedAt
	Day       datatypes.Date
	Clock     datatypes.Time
	Closing   *datatypes.Time
	Count     int
	Tally     sql.NullInt64
	Ratio     sql.NullFloat64
	Flag      sql.NullBool
	Note      string
	Doc       datatypes.JSON
	DocText   string         `gorm:"type:json"`
	DocBytes  []byte         `gorm:"type:jsonb"`
	DocTagged datatypes.JSON `gorm:"type:text"`
	Tags      []string       `gorm:"serializer:json"`
}

// columnsWhere lists, in name order, the database names of the
// sampleStoredRecord columns a predicate holds for.
func columnsWhere(t *testing.T, predicate func(modelschema.Column) bool) []string {
	t.Helper()
	columns, err := modelschema.Columns(reflect.TypeFor[sampleStoredRecord]())
	require.NoError(t, err)
	names := make([]string, 0)
	for _, c := range columns {
		if predicate(c) {
			names = append(names, c.DBName)
		}
	}
	return names
}

// TestColumnPredicatesReadGormDataTypes pins that the runtime predicates
// classify a column by the data types gorm reads for its field, not by its
// Go shape.
func TestColumnPredicatesReadGormDataTypes(t *testing.T) {
	t.Run("Time", func(t *testing.T) {
		require.Equal(t, []string{"at", "day", "removed", "settled"}, columnsWhere(t, modelschema.IsTimeColumn),
			"an instant whatever type its tag declares, a day, and the times Valuer wrappers hold")
	})

	t.Run("TimeOfDay", func(t *testing.T) {
		require.Equal(t, []string{"clock", "closing"}, columnsWhere(t, modelschema.IsTimeOfDayColumn),
			"a pointer column holds the pointed-to time of day")
	})

	t.Run("Numeric", func(t *testing.T) {
		require.Equal(t, []string{"count", "ratio", "tally"}, columnsWhere(t, modelschema.IsNumericColumn),
			"the numbers Valuer wrappers hold count, a time of day does not, whatever its Go kind")
	})

	t.Run("JSON", func(t *testing.T) {
		require.Equal(t, []string{"doc", "doc_bytes", "doc_tagged", "doc_text"}, columnsWhere(t, modelschema.IsJSONColumn),
			"a JSON type whatever type its tag declares, and a column a type tag declares JSON; a serializer stores text")
	})
}

func TestIsDateType(t *testing.T) {
	require.True(t, modelschema.IsDateType(reflect.TypeFor[datatypes.Date]()))
	require.True(t, modelschema.IsDateType(reflect.TypeFor[*datatypes.Date]()), "a pointer column holds the pointed-to day")
	require.False(t, modelschema.IsDateType(reflect.TypeFor[time.Time]()), "an instant is not a calendar day")
	require.False(t, modelschema.IsDateType(reflect.TypeFor[string]()))
	require.False(t, modelschema.IsDateType(nil))
}

func TestUTCDay(t *testing.T) {
	// Midnight in Shanghai on the 2nd is still the 1st in UTC.
	sent := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.FixedZone("", 8*3600))
	require.Equal(t, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), modelschema.UTCDay(sent))
	// The time of day is dropped, the day kept.
	require.Equal(t, time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC),
		modelschema.UTCDay(time.Date(2026, time.January, 2, 23, 59, 59, 999, time.UTC)))
	require.True(t, modelschema.UTCDay(time.Time{}).IsZero(), "the zero time stays zero")
}

// sampleStringsDocument is a custom JSON wrapper over a slice of strings: it
// declares its column type the way the datatypes family does.
type sampleStringsDocument []string

func (sampleStringsDocument) GormDataType() string { return "json" }

// TestIsJSONStringsType pins which column types store a JSON array of
// strings: a JSON document type whose Go shape is a slice of strings,
// pointers dereferenced, and nothing else.
func TestIsJSONStringsType(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[datatypes.JSONSlice[string]](),
		reflect.TypeFor[*datatypes.JSONSlice[string]](),
		reflect.TypeFor[sampleStringsDocument](),
	} {
		require.True(t, modelschema.IsJSONStringsType(typ), typ.String())
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[datatypes.JSONSlice[int]](), reflect.TypeFor[datatypes.JSONMap](),
		reflect.TypeFor[datatypes.JSON](), reflect.TypeFor[datatypes.JSONType[[]string]](),
		reflect.TypeFor[[]string](), reflect.TypeFor[string](),
	} {
		require.False(t, modelschema.IsJSONStringsType(typ), typ.String())
	}
	require.False(t, modelschema.IsJSONStringsType(nil))
}
