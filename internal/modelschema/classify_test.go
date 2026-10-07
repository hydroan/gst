package modelschema_test

import (
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

	t.Run("Numeric", func(t *testing.T) {
		for _, typ := range []reflect.Type{
			reflect.TypeFor[int](), reflect.TypeFor[int8](), reflect.TypeFor[int16](),
			reflect.TypeFor[int32](), reflect.TypeFor[int64](),
			reflect.TypeFor[uint](), reflect.TypeFor[uint8](), reflect.TypeFor[uint16](),
			reflect.TypeFor[uint32](), reflect.TypeFor[uint64](),
			reflect.TypeFor[float32](), reflect.TypeFor[float64](),
			// A named numeric type is still numeric: its kind is what SUM acts on.
			reflect.TypeFor[namedAmount](),
			// A pointer column aggregates its pointed-to value.
			reflect.TypeFor[*int64](),
		} {
			require.Equal(t, modelschema.ColumnClassNumeric, modelschema.ClassifyColumn(typ), typ.String())
		}
	})

	t.Run("Time", func(t *testing.T) {
		require.Equal(t, modelschema.ColumnClassTime, modelschema.ClassifyColumn(reflect.TypeFor[time.Time]()))
		require.Equal(t, modelschema.ColumnClassTime, modelschema.ClassifyColumn(reflect.TypeFor[*time.Time]()))
		// A calendar day is stored in a date column and binds as a time at
		// midnight, so it compares and buckets like an instant.
		require.Equal(t, modelschema.ColumnClassTime, modelschema.ClassifyColumn(reflect.TypeFor[datatypes.Date]()))
		require.Equal(t, modelschema.ColumnClassTime, modelschema.ClassifyColumn(reflect.TypeFor[*datatypes.Date]()))
	})

	t.Run("Other", func(t *testing.T) {
		for _, typ := range []reflect.Type{
			reflect.TypeFor[string](), reflect.TypeFor[bool](),
			reflect.TypeFor[namedLabel](), reflect.TypeFor[[]byte](),
			// A named type whose underlying type is time.Time is neither of
			// the two time types the framework knows how to bind and read
			// back, time.Time and datatypes.Date.
			reflect.TypeFor[namedTime](),
		} {
			require.Equal(t, modelschema.ColumnClassOther, modelschema.ClassifyColumn(typ), typ.String())
		}
		require.Equal(t, modelschema.ColumnClassOther, modelschema.ClassifyColumn(nil))
	})

	t.Run("RejectsValuerHeuristic", func(t *testing.T) {
		// Every type here implements driver.Valuer while being stored as text.
		// Classifying by that interface instead of by kind would call them
		// numeric, and MySQL and SQLite answer SUM over text with 0 rather than
		// an error, so the mistake would reach a report as a wrong number.
		for _, typ := range []reflect.Type{
			reflect.TypeFor[textValuer](), reflect.TypeFor[gorm.DeletedAt](),
		} {
			require.Equal(t, modelschema.ColumnClassOther, modelschema.ClassifyColumn(typ), typ.String())
		}
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

// pointerDataType declares its gorm data type on the pointer receiver, the
// way a custom wrapper with pointer-based Scan/Value methods would.
type pointerDataType struct{}

func (*pointerDataType) GormDataType() string { return "JSONB" }

func TestIsJSONType(t *testing.T) {
	t.Run("JSON", func(t *testing.T) {
		for _, typ := range []reflect.Type{
			reflect.TypeFor[datatypes.JSON](),
			reflect.TypeFor[datatypes.JSONType[map[string]string]](),
			reflect.TypeFor[datatypes.JSONSlice[string]](),
			reflect.TypeFor[datatypes.JSONMap](),
			// A pointer column stores the pointed-to document.
			reflect.TypeFor[*datatypes.JSONSlice[string]](),
			// The declaration may hang off the pointer receiver, and the
			// spelling is case-insensitive.
			reflect.TypeFor[pointerDataType](),
		} {
			require.True(t, modelschema.IsJSONType(typ), typ.String())
		}
	})

	t.Run("NotJSON", func(t *testing.T) {
		for _, typ := range []reflect.Type{
			reflect.TypeFor[string](), reflect.TypeFor[[]string](),
			reflect.TypeFor[[]byte](), reflect.TypeFor[gorm.DeletedAt](),
			reflect.TypeFor[time.Time](),
		} {
			require.False(t, modelschema.IsJSONType(typ), typ.String())
		}
		require.False(t, modelschema.IsJSONType(nil))
	})
}
