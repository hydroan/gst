package modelschema_test

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
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
	Active    bool
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

	t.Run("Bool", func(t *testing.T) {
		require.Equal(t, []string{"active", "flag"}, columnsWhere(t, modelschema.IsBoolColumn),
			"a boolean, and the one sql.NullBool holds")
	})

	t.Run("JSON", func(t *testing.T) {
		require.Equal(t, []string{"doc", "doc_bytes", "doc_tagged", "doc_text"}, columnsWhere(t, modelschema.IsJSONColumn),
			"a JSON type whatever type its tag declares, and a column a type tag declares JSON; a serializer stores text")
	})
}

// TestParseTime pins the one reading of a time a filter or a cursor takes:
// RFC 3339 with its offset, fractional seconds included, and nothing
// zone-less, digit-only or otherwise spelled.
func TestParseTime(t *testing.T) {
	t.Run("RFC3339UTC", func(t *testing.T) {
		got, err := modelschema.ParseTime("2026-07-01T08:30:15Z")
		require.NoError(t, err)
		require.True(t, got.Equal(time.Date(2026, 7, 1, 8, 30, 15, 0, time.UTC)))
	})

	t.Run("RFC3339KeepsExplicitOffset", func(t *testing.T) {
		got, err := modelschema.ParseTime("2026-07-01T08:30:15+08:00")
		require.NoError(t, err)
		require.True(t, got.Equal(time.Date(2026, 7, 1, 8, 30, 15, 0, time.FixedZone("", 8*3600))))
	})

	t.Run("RFC3339FractionalSeconds", func(t *testing.T) {
		got, err := modelschema.ParseTime("2026-07-01T08:30:15.123456789Z")
		require.NoError(t, err)
		require.True(t, got.Equal(time.Date(2026, 7, 1, 8, 30, 15, 123456789, time.UTC)))
	})

	t.Run("ZoneLessValueFails", func(t *testing.T) {
		// Every zone-less spelling names a different instant per server zone,
		// so all of them are rejected: RFC 3339 with its mandatory offset is
		// the one accepted format.
		for _, value := range []string{
			"2026-07-01 08:30:15",
			"2026-07-01T08:30:15",
			"2026-07-01 08:30",
			"2026-07-01T08:30",
			"2026-07-01",
		} {
			_, err := modelschema.ParseTime(value)
			require.Error(t, err, "zone-less value %q must be rejected", value)
		}
	})

	t.Run("UnixTimestampFails", func(t *testing.T) {
		for _, value := range []string{"1751328000", "1751328000123"} {
			_, err := modelschema.ParseTime(value)
			require.Error(t, err, "digit-only value %q must be rejected", value)
		}
	})

	t.Run("UnsupportedFormatFails", func(t *testing.T) {
		_, err := modelschema.ParseTime("07/01/2026")
		require.Error(t, err)
	})
}

// TestParseTimeOfDay pins the one reading of a time of day the framework
// takes, in a filter and in a request body alike: time.TimeOnly, a
// fractional second read as written, answered as the duration since
// midnight, and anything else refused.
func TestParseTimeOfDay(t *testing.T) {
	for value, want := range map[string]time.Duration{
		"17:30:00":           17*time.Hour + 30*time.Minute,
		"17:30:00.5":         17*time.Hour + 30*time.Minute + 500*time.Millisecond,
		"23:59:59.999":       24*time.Hour - time.Millisecond,
		"00:00:00.000000001": time.Nanosecond,
	} {
		got, err := modelschema.ParseTimeOfDay(value)
		require.NoError(t, err, value)
		require.Equal(t, want, got, value)
	}
	for _, value := range []string{"abc", "17:30", "1730", "25:00:00", "17:60:00", ""} {
		_, err := modelschema.ParseTimeOfDay(value)
		require.EqualError(t, err, fmt.Sprintf("unsupported time of day format %q, expect 15:04:05", value), value)
	}
}

// TestValidateNumericValue pins that a value is checked against the kind of
// number its column stores by the data type gorm reads for the field: an
// integer, an unsigned one or a float, the number a Valuer holds included;
// a column storing no number takes any value.
func TestValidateNumericValue(t *testing.T) {
	type counted struct {
		ID    string `gorm:"primaryKey"`
		Count int
		Size  uint
		Ratio float64
		Tally sql.NullInt64
		Note  string
	}
	columns, err := modelschema.Columns(reflect.TypeFor[counted]())
	require.NoError(t, err)
	byName := make(map[string]modelschema.Column, len(columns))
	for _, c := range columns {
		byName[c.DBName] = c
	}
	for name, values := range map[string]struct{ taken, refused []string }{
		"count": {taken: []string{"-3", "42"}, refused: []string{"1.5", "x", ""}},
		"size":  {taken: []string{"0", "42"}, refused: []string{"-1", "x"}},
		"ratio": {taken: []string{"1.5", "-2", "1e3"}, refused: []string{"x"}},
		"tally": {taken: []string{"7"}, refused: []string{"7.5"}},
		"note":  {taken: []string{"x", "1.5"}},
	} {
		for _, value := range values.taken {
			require.NoError(t, modelschema.ValidateNumericValue(byName[name], value), "%s %q", name, value)
		}
		for _, value := range values.refused {
			require.EqualError(t, modelschema.ValidateNumericValue(byName[name], value), fmt.Sprintf("expect a numeric value, got %q", value), "%s %q", name, value)
		}
	}
}

// TestParseBool pins the one reading of a boolean the framework takes
// from a client: the spellings strconv.ParseBool accepts, and a refusal
// naming the value for anything else.
func TestParseBool(t *testing.T) {
	for value, want := range map[string]bool{"true": true, "1": true, "T": true, "false": false, "0": false, "F": false} {
		got, err := modelschema.ParseBool(value)
		require.NoError(t, err, value)
		require.Equal(t, want, got, value)
	}
	for _, value := range []string{"maybe", "yes", ""} {
		_, err := modelschema.ParseBool(value)
		require.EqualError(t, err, fmt.Sprintf("expect a boolean value, got %q", value), value)
	}
}

// TestIsDateColumn pins which columns store a calendar day: the column type
// gorm resolved for them is date, declared by the Go type or by a type tag in
// either case, a pointer included, and DateColumnSet names the same columns;
// an instant and a column declared as another time type are no date.
func TestIsDateColumn(t *testing.T) {
	type dated struct {
		Day     datatypes.Date  `json:"day"`
		DayPtr  *datatypes.Date `json:"day_ptr"`
		Due     time.Time       `json:"due" gorm:"type:date"`
		DuePtr  *time.Time      `json:"due_ptr" gorm:"type:DATE"`
		At      time.Time       `json:"at"`
		Stamped time.Time       `json:"stamped" gorm:"type:datetime(3)"`
		Clock   datatypes.Time  `json:"clock"`
	}
	columns, err := modelschema.Columns(reflect.TypeFor[dated]())
	require.NoError(t, err)
	got := make(map[string]bool, len(columns))
	for _, c := range columns {
		got[c.DBName] = modelschema.IsDateColumn(c)
	}
	require.Equal(t, map[string]bool{"day": true, "day_ptr": true, "due": true, "due_ptr": true, "at": false, "stamped": false, "clock": false}, got)
	require.Equal(t, map[string]struct{}{"day": {}, "day_ptr": {}, "due": {}, "due_ptr": {}}, modelschema.DateColumnSet(reflect.TypeFor[dated]()))
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
