package datatypes_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/datatypes"
	"github.com/stretchr/testify/require"
	gormdatatypes "gorm.io/datatypes"
)

// TestTimeOfDayWritesAndReadsATimeAsItsText pins the text of a TimeOfDay,
// the examples of its comment among them: 17:30:00 as a JSON string and
// through String, a fraction of a second written in nine digits and read as
// written, a nil pointer written as null, and a null leaving a time of day
// as it is.
func TestTimeOfDayWritesAndReadsATimeAsItsText(t *testing.T) {
	type timed struct {
		Opens datatypes.TimeOfDay  `json:"opens"`
		Shuts *datatypes.TimeOfDay `json:"shuts"`
	}
	opens := datatypes.NewTimeOfDay(17, 30, 0, 0)
	require.Equal(t, "17:30:00", opens.String())
	require.Equal(t, "17:30:00.500000000", datatypes.NewTimeOfDay(17, 30, 0, 500_000_000).String())
	require.Equal(t, 17*time.Hour+30*time.Minute, time.Duration(opens))

	written, err := json.Marshal(timed{Opens: opens})
	require.NoError(t, err)
	require.JSONEq(t, `{"opens":"17:30:00","shuts":null}`, string(written))

	var read timed
	require.NoError(t, json.Unmarshal([]byte(`{"opens":"08:00:00","shuts":"17:30:00.5"}`), &read))
	require.Equal(t, datatypes.NewTimeOfDay(8, 0, 0, 0), read.Opens)
	require.Equal(t, datatypes.NewTimeOfDay(17, 30, 0, 500_000_000), *read.Shuts)

	require.NoError(t, json.Unmarshal([]byte(`{"opens":null,"shuts":null}`), &read))
	require.Equal(t, "08:00:00", read.Opens.String(), "a null leaves the time of day as it is")
	require.Nil(t, read.Shuts)
}

// TestTimeOfDayRefusesWhatIsNoTimeOfDay pins what a TimeOfDay refuses in
// JSON and ParseTimeOfDay in a filter, the examples of ParseTimeOfDay's
// comment among them: a word, a time with no seconds, an hour no day has, a
// time with a zone, and a value that is no string; the library's type,
// which the value is stored through, reads a word as midnight.
func TestTimeOfDayRefusesWhatIsNoTimeOfDay(t *testing.T) {
	for text, want := range map[string]datatypes.TimeOfDay{
		"17:30:00":           datatypes.NewTimeOfDay(17, 30, 0, 0),
		"17:30:00.5":         datatypes.NewTimeOfDay(17, 30, 0, 500_000_000),
		"23:59:59.999":       datatypes.NewTimeOfDay(23, 59, 59, 999_000_000),
		"00:00:00.000000001": datatypes.NewTimeOfDay(0, 0, 0, 1),
	} {
		got, parseErr := datatypes.ParseTimeOfDay(text)
		require.NoError(t, parseErr, text)
		require.Equal(t, want, got, text)
	}

	var err error
	for _, text := range []string{"abc", "17:30", "1730", "25:00:00", "17:60:00", "17:30:00Z", "", " 17:30:00"} {
		_, err = datatypes.ParseTimeOfDay(text)
		require.EqualError(t, err, `unsupported time of day format "`+text+`", expect 15:04:05`)

		var read datatypes.TimeOfDay
		quoted, quoteErr := json.Marshal(text)
		require.NoError(t, quoteErr)
		require.EqualError(t, json.Unmarshal(quoted, &read), `unsupported time of day format "`+text+`", expect 15:04:05`)
	}
	for _, value := range []string{`1730`, `true`, `{"hour":17}`, `["17:30:00"]`} {
		var read datatypes.TimeOfDay
		require.ErrorContains(t, json.Unmarshal([]byte(value), &read), "a time of day is a JSON string, as 15:04:05", value)
	}

	var lenient gormdatatypes.Time
	require.NoError(t, json.Unmarshal([]byte(`"abc"`), &lenient))
	require.Zero(t, lenient, "the library's time reads a word as midnight")
}

// TestTimeOfDayStoresThroughTheLibrarysTime pins that a time column takes
// and gives a TimeOfDay the way it takes and gives the Time of
// gorm.io/datatypes: the value it is written as, the texts and the time it
// is read from, and the data type it declares.
func TestTimeOfDayStoresThroughTheLibrarysTime(t *testing.T) {
	half := datatypes.NewTimeOfDay(17, 30, 0, 500_000_000)

	column, err := half.Value()
	require.NoError(t, err)
	library, err := gormdatatypes.Time(half).Value()
	require.NoError(t, err)
	require.Equal(t, library, column)
	require.Equal(t, "17:30:00.500000000", column)

	for _, stored := range []any{"17:30:00.500000000", []byte("17:30:00.500000000"), time.Date(2026, time.January, 2, 17, 30, 0, 500_000_000, time.UTC)} {
		var scanned datatypes.TimeOfDay
		require.NoError(t, scanned.Scan(stored))
		require.Equal(t, half, scanned)
	}
	var scanned datatypes.TimeOfDay
	require.Error(t, scanned.Scan(1730))

	require.Equal(t, gormdatatypes.Time(0).GormDataType(), half.GormDataType())
}
