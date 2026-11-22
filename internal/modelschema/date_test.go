package modelschema_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/modelschema"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// TestDateWritesAndReadsADayAsItsText pins the text of a Date, the examples
// of its comment among them: 2026-01-02 as a JSON string and through String,
// read back as the same day at midnight UTC, a nil pointer written as null,
// and a null leaving a day as it is.
func TestDateWritesAndReadsADayAsItsText(t *testing.T) {
	type dated struct {
		Day modelschema.Date  `json:"day"`
		Due *modelschema.Date `json:"due"`
	}
	day := modelschema.Date(time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC))
	require.Equal(t, "2026-01-02", day.String())

	written, err := json.Marshal(dated{Day: day})
	require.NoError(t, err)
	require.JSONEq(t, `{"day":"2026-01-02","due":null}`, string(written))

	var read dated
	require.NoError(t, json.Unmarshal([]byte(`{"day":"2026-01-02","due":"2026-03-04"}`), &read))
	require.True(t, time.Time(read.Day).Equal(time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)))
	require.Equal(t, time.UTC, time.Time(read.Day).Location())
	require.Equal(t, "2026-03-04", read.Due.String())

	require.NoError(t, json.Unmarshal([]byte(`{"day":null,"due":null}`), &read))
	require.Equal(t, "2026-01-02", read.Day.String(), "a null leaves the day as it is")
	require.Nil(t, read.Due)
}

// TestDateRefusesWhatIsNoDay pins what a Date refuses in JSON and ParseDate
// in a filter, the example of ParseDate's comment among them: an instant,
// whatever its offset, a day written loosely, a day the calendar does not
// hold, and a value that is no string.
func TestDateRefusesWhatIsNoDay(t *testing.T) {
	day, err := modelschema.ParseDate("2026-01-02")
	require.NoError(t, err)
	require.Equal(t, "2026-01-02", day.String())

	for _, text := range []string{"2026-01-02T00:00:00+08:00", "2026-01-02T00:00:00Z", "2026-1-2", "2026-02-30", "20260102", "", " 2026-01-02"} {
		_, err = modelschema.ParseDate(text)
		require.EqualError(t, err, `unsupported date format "`+text+`", expect 2006-01-02`)

		var read modelschema.Date
		quoted, quoteErr := json.Marshal(text)
		require.NoError(t, quoteErr)
		require.EqualError(t, json.Unmarshal(quoted, &read), `unsupported date format "`+text+`", expect 2006-01-02`)
	}
	for _, value := range []string{`20260102`, `true`, `{"year":2026}`, `["2026-01-02"]`} {
		var read modelschema.Date
		require.ErrorContains(t, json.Unmarshal([]byte(value), &read), "a date is a JSON string, as 2006-01-02", value)
	}
}

// TestDateIsTheDayItsTimeReads pins that the day is the one the time reads
// in its own location, in its text and in the value a date column takes, its
// midnight in UTC: a morning in Shanghai is that day, where the library's
// date writes the midnight of Shanghai, the evening before in UTC.
func TestDateIsTheDayItsTimeReads(t *testing.T) {
	shanghai := time.FixedZone("UTC+8", 8*60*60)
	morning := time.Date(2026, time.January, 2, 7, 0, 0, 0, shanghai)

	day := modelschema.Date(morning)
	require.Equal(t, "2026-01-02", day.String())
	written, err := json.Marshal(day)
	require.NoError(t, err)
	require.JSONEq(t, `"2026-01-02"`, string(written))

	column, err := day.Value()
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC), column)

	library, err := datatypes.Date(morning).Value()
	require.NoError(t, err)
	stored, ok := library.(time.Time)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, time.January, 1, 16, 0, 0, 0, time.UTC), stored.UTC(), "the library's date is the midnight of the time's own location")

	var scanned modelschema.Date
	require.NoError(t, scanned.Scan(time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)))
	require.Equal(t, "2026-01-02", scanned.String())
}

// TestValidateDateColumnsHoldsDateColumnsToDate pins the rule and the error
// ValidateDateColumns documents: a date column declared by the Date of
// gorm.io/datatypes, or by a time.Time a type tag declares a date, is
// reported by its field and its type, every such column in one error,
// whatever the spelling of the tag and the type under it; a model declaring
// its days as Date, a pointer to one among them, and one with no date column
// have nothing to report.
func TestValidateDateColumnsHoldsDateColumnsToDate(t *testing.T) {
	type Record struct {
		Day datatypes.Date `json:"day"`
		Due *time.Time     `json:"due" gorm:"type:date"`
		At  time.Time      `json:"at"`
	}
	err := modelschema.ValidateDateColumns(reflect.TypeFor[*Record]())
	require.EqualError(t, err, "model Record: field Day stores a date as datatypes.Date; field Due stores a date as *time.Time; a date column is declared model.Date, or *model.Date when it is optional")

	type shouted struct {
		Day string `json:"day" gorm:"type:DATE"`
	}
	require.ErrorContains(t, modelschema.ValidateDateColumns(reflect.TypeFor[shouted]()), "field Day stores a date as string")

	type dated struct {
		Day modelschema.Date  `json:"day"`
		Due *modelschema.Date `json:"due"`
		At  time.Time         `json:"at"`
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
		Day modelschema.Date  `json:"day"`
		Due *modelschema.Date `json:"due"`
		At  time.Time         `json:"at"`
	}
	columns, err := modelschema.Columns(reflect.TypeFor[dated]())
	require.NoError(t, err)
	got := make(map[string]bool, len(columns))
	for _, c := range columns {
		got[c.DBName] = modelschema.IsDateColumn(c)
	}
	require.Equal(t, map[string]bool{"day": true, "due": true, "at": false}, got)
}
