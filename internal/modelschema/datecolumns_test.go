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

// TestValidateDateColumnsHoldsDateColumnsToDate pins the rule and the error
// ValidateDateColumns documents: a date column declared by the Date of
// gorm.io/datatypes, or by a time.Time a type tag declares a date, is
// reported by its field and its type, every such column in one error,
// whatever the spelling of the tag and the type under it; a model declaring
// its days as Date, a pointer to one among them, and one with no date column
// have nothing to report.
func TestValidateDateColumnsHoldsDateColumnsToDate(t *testing.T) {
	type Record struct {
		Day gormdatatypes.Date `json:"day"`
		Due *time.Time         `json:"due" gorm:"type:date"`
		At  time.Time          `json:"at"`
	}
	err := modelschema.ValidateDateColumns(reflect.TypeFor[*Record]())
	require.EqualError(t, err, "model Record: field Day stores a date as datatypes.Date; field Due stores a date as *time.Time; a date column is declared gst.Date, or *gst.Date when it is optional")

	type shouted struct {
		Day string `json:"day" gorm:"type:DATE"`
	}
	require.ErrorContains(t, modelschema.ValidateDateColumns(reflect.TypeFor[shouted]()), "field Day stores a date as string")

	type dated struct {
		Day datatypes.Date  `json:"day"`
		Due *datatypes.Date `json:"due"`
		At  time.Time       `json:"at"`
	}
	require.NoError(t, modelschema.ValidateDateColumns(reflect.TypeFor[dated]()))
	type undated struct {
		At time.Time `json:"at"`
	}
	require.NoError(t, modelschema.ValidateDateColumns(reflect.TypeFor[*undated]()))
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
