package modelschema_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/datatypes"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/stretchr/testify/require"
	gormdatatypes "gorm.io/datatypes"
)

// TestValidateColumnTypesHoldsColumnsToTheFrameworksTypes pins the rule and
// the error ValidateColumnTypes documents: a column declared by a type of
// gorm.io/datatypes the framework has its own for, a pointer to one among
// them, and a date column declared by a time.Time a type tag declares a
// date, are each reported by the field and the declaration to write, every
// such column in one error, whatever the spelling of the tag and the type
// under it; a type of the library the framework has none for, a model
// declaring its columns by the framework's types, and one with no such
// column have nothing to report.
func TestValidateColumnTypesHoldsColumnsToTheFrameworksTypes(t *testing.T) {
	type Record struct {
		Day  gormdatatypes.Date              `json:"day"`
		Due  *time.Time                      `json:"due" gorm:"type:date"`
		Tags gormdatatypes.JSONSlice[string] `json:"tags"`
		At   time.Time                       `json:"at"`
	}
	err := modelschema.ValidateColumnTypes(reflect.TypeFor[*Record]())
	require.EqualError(t, err, "model Record: field Day is declared gorm.io/datatypes.Date, declare it datatypes.Date; field Due is a date column declared *time.Time, declare it *datatypes.Date; field Tags is declared gorm.io/datatypes.JSONSlice, declare it datatypes.JSONSlice; datatypes is github.com/hydroan/gst/datatypes")

	type library struct {
		Clock   *gormdatatypes.Time                    `json:"clock"`
		Raw     gormdatatypes.JSON                     `json:"raw"`
		Meta    gormdatatypes.JSONMap                  `json:"meta"`
		Options gormdatatypes.JSONType[map[string]int] `json:"options"`
		Link    gormdatatypes.URL                      `json:"link"`
	}
	require.EqualError(t, modelschema.ValidateColumnTypes(reflect.TypeFor[library]()), "model library: field Clock is declared *gorm.io/datatypes.Time, declare it *datatypes.TimeOfDay; field Meta is declared gorm.io/datatypes.JSONMap, declare it datatypes.JSONMap; field Options is declared gorm.io/datatypes.JSONType, declare it datatypes.JSON; field Raw is declared gorm.io/datatypes.JSON, declare it datatypes.JSON[json.RawMessage]; datatypes is github.com/hydroan/gst/datatypes")

	type shouted struct {
		Day string `json:"day" gorm:"type:DATE"`
	}
	require.ErrorContains(t, modelschema.ValidateColumnTypes(reflect.TypeFor[shouted]()), "field Day is a date column declared string, declare it datatypes.Date")

	type owned struct {
		Day     datatypes.Date                 `json:"day"`
		Due     *datatypes.Date                `json:"due"`
		Clock   datatypes.TimeOfDay            `json:"clock"`
		Tags    datatypes.JSONSlice[string]    `json:"tags"`
		Meta    datatypes.JSONMap              `json:"meta"`
		Options datatypes.JSON[map[string]int] `json:"options"`
		At      time.Time                      `json:"at"`
	}
	require.NoError(t, modelschema.ValidateColumnTypes(reflect.TypeFor[owned]()))
	type plain struct {
		At time.Time `json:"at"`
	}
	require.NoError(t, modelschema.ValidateColumnTypes(reflect.TypeFor[*plain]()))
}

// TestDateDeclaresADateColumn pins that a Date and a pointer to one are date
// columns to the framework, and an instant is none.
func TestDateDeclaresADateColumn(t *testing.T) {
	type dated struct {
		Day datatypes.Date  `json:"day"`
		Due *datatypes.Date `json:"due"`
		At  time.Time       `json:"at"`
	}
	columns, err := modelschema.Columns(reflect.TypeFor[dated]())
	require.NoError(t, err)
	got := make(map[string]bool, len(columns))
	for _, c := range columns {
		got[c.DBName] = modelschema.IsDateColumn(c)
	}
	require.Equal(t, map[string]bool{"day": true, "due": true, "at": false}, got)
}
