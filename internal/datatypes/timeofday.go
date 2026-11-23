package datatypes

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/cockroachdb/errors"
	gormdatatypes "gorm.io/datatypes"
	"gorm.io/gorm"
	gormschema "gorm.io/gorm/schema"
)

// TimeOfDay is a time of day, the time since midnight a clock shows: when a
// counter opens, when a job is due. It names no day and carries no zone, so
// a time of day is written the same wherever it is read: 17:30:00 in a
// request body, in a response and in a URL filter (see ParseTimeOfDay), with
// the nine digits of its fraction of a second when it has one,
// 17:30:00.500000000, and a google.type.TimeOfDay over gRPC. A model
// declares a time of day as a TimeOfDay and an optional one as a
// *TimeOfDay, and its table stores either in a time column, text on SQLite.
//
// Only the reading of its text is the type's own. The column, and what is
// written to it and read from it, are those of the Time of
// gorm.io/datatypes, which the value is stored through by conversion; what
// the library's type cannot be told is to refuse a text it does not read:
// it scans a string leniently and keeps no error, "abc" read as midnight.
type TimeOfDay time.Duration //nolint:recvcheck // UnmarshalJSON and Scan write through the pointer, MarshalJSON and Value read the value: the receiver pairs encoding/json and database/sql document.

// NewTimeOfDay returns the time of day hour hours, minute minutes, second
// seconds and nanosecond nanoseconds after midnight.
func NewTimeOfDay(hour, minute, second, nanosecond int) TimeOfDay {
	return TimeOfDay(gormdatatypes.NewTime(hour, minute, second, nanosecond))
}

// ParseTimeOfDay reads a time of day the one way the framework takes it, in
// a request body and in a URL filter alike: time.TimeOnly, 15:04:05, a
// fraction of a second allowed and read as written, so 17:30:00.5 is half a
// second past 17:30. Anything else is refused, "abc", "17:30" and "25:00:00"
// among it.
func ParseTimeOfDay(value string) (TimeOfDay, error) {
	t, err := time.Parse(time.TimeOnly, value)
	if err != nil {
		return 0, errors.Newf("unsupported time of day format %q, expect %s", value, time.TimeOnly)
	}
	return NewTimeOfDay(t.Hour(), t.Minute(), t.Second(), t.Nanosecond()), nil
}

// String returns the time of day as 15:04:05, followed by the nine digits
// of its fraction of a second when it has one.
func (t TimeOfDay) String() string {
	return gormdatatypes.Time(t).String()
}

// MarshalJSON writes the time of day as a JSON string, "17:30:00".
func (t TimeOfDay) MarshalJSON() ([]byte, error) {
	return gormdatatypes.Time(t).MarshalJSON()
}

// UnmarshalJSON reads a time of day written as a JSON string, "17:30:00"
// (see ParseTimeOfDay), and refuses every other value. A null leaves the
// time of day as it is, the way encoding/json leaves a value a null is
// decoded into.
func (t *TimeOfDay) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return errors.New("a time of day is a JSON string, as 15:04:05")
	}
	parsed, err := ParseTimeOfDay(text)
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// Scan reads the time of day a time column holds, the way
// gormdatatypes.Time reads it.
func (t *TimeOfDay) Scan(value any) error {
	return (*gormdatatypes.Time)(t).Scan(value)
}

// Value writes the time of day for a time column, the way
// gormdatatypes.Time writes it.
func (t TimeOfDay) Value() (driver.Value, error) {
	return gormdatatypes.Time(t).Value()
}

// GormDataType declares the data type gormdatatypes.Time declares.
func (t TimeOfDay) GormDataType() string {
	return gormdatatypes.Time(t).GormDataType()
}

// GormDBDataType declares the time column of the dialect db speaks, the one
// gormdatatypes.Time declares.
func (t TimeOfDay) GormDBDataType(db *gorm.DB, field *gormschema.Field) string {
	return gormdatatypes.Time(t).GormDBDataType(db, field)
}
