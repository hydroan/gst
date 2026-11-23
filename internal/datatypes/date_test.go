package datatypes_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/datatypes"
	"github.com/stretchr/testify/require"
	gormdatatypes "gorm.io/datatypes"
)

// TestDateWritesAndReadsADayAsItsText pins the text of a Date, the examples
// of its comment among them: 2026-01-02 as a JSON string and through String,
// read back as the same day at midnight UTC, a nil pointer written as null,
// and a null leaving a day as it is.
func TestDateWritesAndReadsADayAsItsText(t *testing.T) {
	type dated struct {
		Day datatypes.Date  `json:"day"`
		Due *datatypes.Date `json:"due"`
	}
	day := datatypes.Date(time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC))
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
	day, err := datatypes.ParseDate("2026-01-02")
	require.NoError(t, err)
	require.Equal(t, "2026-01-02", day.String())

	for _, text := range []string{"2026-01-02T00:00:00+08:00", "2026-01-02T00:00:00Z", "2026-1-2", "2026-02-30", "20260102", "", " 2026-01-02"} {
		_, err = datatypes.ParseDate(text)
		require.EqualError(t, err, `unsupported date format "`+text+`", expect 2006-01-02`)

		var read datatypes.Date
		quoted, quoteErr := json.Marshal(text)
		require.NoError(t, quoteErr)
		require.EqualError(t, json.Unmarshal(quoted, &read), `unsupported date format "`+text+`", expect 2006-01-02`)
	}
	for _, value := range []string{`20260102`, `true`, `{"year":2026}`, `["2026-01-02"]`} {
		var read datatypes.Date
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

	day := datatypes.Date(morning)
	require.Equal(t, "2026-01-02", day.String())
	written, err := json.Marshal(day)
	require.NoError(t, err)
	require.JSONEq(t, `"2026-01-02"`, string(written))

	column, err := day.Value()
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC), column)

	library, err := gormdatatypes.Date(morning).Value()
	require.NoError(t, err)
	stored, ok := library.(time.Time)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, time.January, 1, 16, 0, 0, 0, time.UTC), stored.UTC(), "the library's date is the midnight of the time's own location")

	var scanned datatypes.Date
	require.NoError(t, scanned.Scan(time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)))
	require.Equal(t, "2026-01-02", scanned.String())
}

// TestDateComparesAndCountsByTheDay pins the arithmetic of a Date, the
// examples of its comments among them: days added and taken, the count of
// days between two, and the comparisons, each by the day a Date names
// whatever time and location it was made of, which the == of two days tells
// apart; and the zero day.
func TestDateComparesAndCountsByTheDay(t *testing.T) {
	shanghai := time.FixedZone("UTC+8", 8*60*60)
	day := datatypes.Date(time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC))
	evening := datatypes.Date(time.Date(2026, time.January, 2, 23, 0, 0, 0, shanghai))
	next := day.AddDays(1)

	require.Equal(t, "2026-01-03", next.String())
	require.Equal(t, "2026-01-01", day.AddDays(-1).String())
	require.Equal(t, "2026-02-01", day.AddDays(30).String())
	require.Equal(t, "2026-01-03", evening.AddDays(1).String())
	require.Equal(t, time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC), time.Time(evening.AddDays(1)), "a day counted from another is its midnight in UTC")

	require.Equal(t, 1, next.DaysSince(day))
	require.Equal(t, 0, evening.DaysSince(day))
	require.Equal(t, -1, day.DaysSince(next))
	require.Equal(t, 146097, datatypes.Date(time.Date(2426, time.January, 2, 0, 0, 0, 0, time.UTC)).DaysSince(day), "four centuries, further apart than a time.Duration holds")

	require.True(t, day.Equal(evening))
	require.NotEqual(t, day, evening, "the == of two days tells their times and locations apart")
	require.Equal(t, 0, day.Compare(evening))
	require.Equal(t, -1, day.Compare(next))
	require.Equal(t, 1, next.Compare(day))
	require.True(t, day.Before(next))
	require.False(t, day.Before(evening))
	require.True(t, next.After(day))
	require.False(t, evening.After(day))

	require.True(t, datatypes.Date{}.IsZero())
	require.True(t, datatypes.Date(time.Date(1, time.January, 1, 5, 0, 0, 0, shanghai)).IsZero())
	require.False(t, day.IsZero())
}
